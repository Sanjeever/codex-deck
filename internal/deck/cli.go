package deck

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/pelletier/go-toml/v2"
	"github.com/pelletier/go-toml/v2/unstable"
)

type CLI struct{ Home string }

type cliTokens struct {
	IDToken      string `json:"id_token"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	AccountID    string `json:"account_id"`
}
type cliAuth struct {
	AuthMode    string     `json:"auth_mode"`
	APIKey      *string    `json:"OPENAI_API_KEY"`
	Tokens      *cliTokens `json:"tokens"`
	LastRefresh *time.Time `json:"last_refresh"`
}

func (c CLI) config() ([]byte, string, error) {
	b, err := os.ReadFile(filepath.Join(c.Home, "config.toml"))
	if os.IsNotExist(err) {
		return nil, "file", nil
	}
	if err != nil {
		return nil, "", err
	}
	var config map[string]interface{}
	if err = toml.Unmarshal(b, &config); err != nil {
		return nil, "", fmt.Errorf("CLI 配置格式无效: %w", err)
	}
	mode := "file"
	if v, exists := config["cli_auth_credentials_store"]; exists {
		var ok bool
		mode, ok = v.(string)
		if !ok {
			return nil, "", fmt.Errorf("CLI 凭据存储配置不是字符串")
		}
	}
	return b, mode, nil
}

func (c CLI) StoreMode() (string, error) { _, mode, err := c.config(); return mode, err }

func (c CLI) EnableFileStore() error {
	b, mode, err := c.config()
	if err != nil {
		return err
	}
	if mode == "file" {
		return nil
	}
	var parser unstable.Parser
	parser.Reset(b)
	for parser.NextExpression() {
		n := parser.Expression()
		if n.Kind == unstable.Table || n.Kind == unstable.ArrayTable {
			break
		}
		if n.Kind != unstable.KeyValue {
			continue
		}
		keys := n.Key()
		if !keys.Next() || string(keys.Node().Data) != "cli_auth_credentials_store" || !keys.IsLast() {
			continue
		}
		r := n.Value().Raw
		updated := append(bytes.Clone(b[:r.Offset]), []byte(`"file"`)...)
		updated = append(updated, b[r.Offset+r.Length:]...)
		return atomicWrite(filepath.Join(c.Home, "config.toml"), updated)
	}
	if err := parser.Error(); err != nil {
		return err
	}
	return fmt.Errorf("未找到 CLI 凭据存储配置项")
}

func (c CLI) Read() (*cliAuth, Identity, error) {
	b, err := os.ReadFile(filepath.Join(c.Home, "auth.json"))
	if os.IsNotExist(err) {
		return nil, Identity{}, nil
	}
	if err != nil {
		return nil, Identity{}, err
	}
	var auth cliAuth
	if err = json.Unmarshal(b, &auth); err != nil {
		return nil, Identity{}, fmt.Errorf("CLI 认证文件格式无效: %w", err)
	}
	if auth.AuthMode != "" && auth.AuthMode != "chatgpt" {
		return &auth, Identity{}, nil
	}
	if auth.Tokens == nil {
		return &auth, Identity{}, nil
	}
	i, err := identityFromToken(auth.Tokens.IDToken)
	if err != nil {
		return nil, i, err
	}
	if auth.Tokens.AccountID != "" && auth.Tokens.AccountID != i.AccountID {
		return nil, i, fmt.Errorf("CLI 认证工作区身份不匹配")
	}
	return &auth, i, nil
}

func (c CLI) Write(t Tokens) error {
	mode, err := c.StoreMode()
	if err != nil {
		return err
	}
	if mode != "file" {
		return fmt.Errorf("CLI 当前使用 %s 凭据存储，请确认改为文件存储", mode)
	}
	a := cliAuth{AuthMode: "chatgpt", Tokens: &cliTokens{IDToken: t.IDToken, AccessToken: t.AccessToken, RefreshToken: t.RefreshToken, AccountID: t.AccountID}, LastRefresh: &t.LastRefresh}
	b, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(filepath.Join(c.Home, "auth.json"), b)
}

func (c CLI) ImportCurrent(a Account) (Account, bool, error) {
	mode, err := c.StoreMode()
	if err != nil {
		return a, false, err
	}
	if mode != "file" {
		return a, false, nil
	}
	auth, i, err := c.Read()
	if err != nil {
		return a, false, err
	}
	if auth == nil || auth.Tokens == nil || !identityMatches(a, i) {
		return a, false, nil
	}
	t := auth.Tokens
	if t.AccessToken == "" || t.RefreshToken == "" {
		return a, false, fmt.Errorf("CLI 当前账号认证不完整")
	}
	a.Tokens.IDToken = t.IDToken
	a.Tokens.AccessToken = t.AccessToken
	a.Tokens.RefreshToken = t.RefreshToken
	a.Tokens.AccountID = i.AccountID
	if auth.LastRefresh != nil {
		a.Tokens.LastRefresh = *auth.LastRefresh
	}
	claims, err := tokenClaims(t.AccessToken)
	if err != nil {
		return a, false, err
	}
	accessIdentity, err := identityFromToken(t.AccessToken)
	if err != nil {
		return a, false, err
	}
	if !identityMatches(a, accessIdentity) {
		return a, false, fmt.Errorf("CLI 访问令牌身份不匹配")
	}
	exp, err := claims.GetExpirationTime()
	if err != nil {
		return a, false, err
	}
	if exp == nil {
		return a, false, fmt.Errorf("CLI 访问令牌缺少到期时间")
	}
	a.Tokens.ExpiresAt = exp.Time
	a.Email = i.Email
	a.Plan = i.Plan
	return a, true, nil
}

// 刷新期间 CLI 可能重新登录或刷新令牌，只在认证仍相同时写回。
func (c CLI) WriteIfUnchanged(old Account, next Tokens) error {
	mode, err := c.StoreMode()
	if err != nil {
		return err
	}
	if mode != "file" {
		return fmt.Errorf("令牌已保存，CLI 凭据存储已变化，未写回 CLI")
	}
	auth, i, err := c.Read()
	if err != nil {
		return err
	}
	if auth == nil || auth.Tokens == nil || !identityMatches(old, i) || auth.Tokens.AccessToken != old.Tokens.AccessToken || auth.Tokens.RefreshToken != old.Tokens.RefreshToken || auth.Tokens.IDToken != old.Tokens.IDToken {
		return fmt.Errorf("令牌已保存，CLI 认证同时发生变化，未覆盖 CLI 文件")
	}
	return c.Write(next)
}
