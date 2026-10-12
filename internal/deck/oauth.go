package deck

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/oauth2"
)

const OAuthClientID = "app_EMoamEEZ73f0CkXaXp7hrann"
const OAuthRedirect = "http://127.0.0.1:1455/auth/callback"

type OAuth struct {
	Client *http.Client
	Issuer string
}

type Login struct {
	ID       string `json:"id"`
	URL      string `json:"url"`
	ctx      context.Context
	cancel   context.CancelFunc
	server   *http.Server
	result   chan loginResult
	verifier string
	redirect string
	address  string
	waitMu   sync.Mutex
	waited   bool
}
type loginResult struct {
	code string
	err  error
}

func (o OAuth) Start(ctx context.Context) (*Login, error) {
	return o.startAt(ctx, "127.0.0.1:1455", OAuthRedirect)
}

func (o OAuth) startAt(ctx context.Context, address, redirect string) (*Login, error) {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, fmt.Errorf("无法监听登录回调，请释放 1455 端口: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	l := &Login{ID: uuid.NewString(), ctx: ctx, cancel: cancel, result: make(chan loginResult, 1), verifier: oauth2.GenerateVerifier(), redirect: redirect, address: listener.Addr().String()}
	state := oauth2.GenerateVerifier()
	config := oauth2.Config{ClientID: OAuthClientID, Endpoint: oauth2.Endpoint{AuthURL: o.Issuer + "/oauth/authorize"}, RedirectURL: redirect, Scopes: []string{"openid", "profile", "email", "offline_access", "api.connectors.read", "api.connectors.invoke"}}
	l.URL = config.AuthCodeURL(state, oauth2.S256ChallengeOption(l.verifier), oauth2.SetAuthURLParam("id_token_add_organizations", "true"), oauth2.SetAuthURLParam("codex_cli_simplified_flow", "true"), oauth2.SetAuthURLParam("originator", "codex_cli_rs"))
	var once sync.Once
	mux := http.NewServeMux()
	mux.HandleFunc("/auth/callback", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "请求方法无效", 405)
			return
		}
		q := r.URL.Query()
		if q.Get("state") != state {
			http.Error(w, "登录状态不匹配", 400)
			return
		}
		result := loginResult{code: q.Get("code")}
		if q.Get("error") != "" {
			result.err = fmt.Errorf("授权失败: %s", q.Get("error"))
		} else if result.code == "" {
			http.Error(w, "缺少授权码", 400)
			return
		}
		once.Do(func() { l.result <- result })
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("<!doctype html><html lang=zh-CN><title>Codex Deck</title><body>授权回调已收到，请返回 Codex Deck 查看结果。</body></html>"))
	})
	l.server = &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if err := l.server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			once.Do(func() { l.result <- loginResult{err: fmt.Errorf("登录回调失败: %w", err)} })
		}
	}()
	go func() { <-ctx.Done(); _ = l.server.Close() }()
	return l, nil
}

func (l *Login) Cancel() { l.cancel() }

func (o OAuth) Wait(l *Login) (Tokens, Identity, error) {
	l.waitMu.Lock()
	defer l.waitMu.Unlock()
	if l.waited {
		return Tokens{}, Identity{}, fmt.Errorf("登录结果已处理")
	}
	l.waited = true
	defer l.cancel()
	select {
	case <-l.ctx.Done():
		return Tokens{}, Identity{}, fmt.Errorf("登录已取消或超时")
	case result := <-l.result:
		if result.err != nil {
			return Tokens{}, Identity{}, result.err
		}
		form := url.Values{"grant_type": {"authorization_code"}, "client_id": {OAuthClientID}, "code": {result.code}, "redirect_uri": {l.redirect}, "code_verifier": {l.verifier}}
		return o.exchange(l.ctx, []byte(form.Encode()), "application/x-www-form-urlencoded", Tokens{}, false)
	}
}

type tokenResponse struct {
	IDToken      string `json:"id_token"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
}

func (o OAuth) Refresh(ctx context.Context, old Tokens) (Tokens, Identity, error) {
	body, err := json.Marshal(map[string]string{"grant_type": "refresh_token", "client_id": OAuthClientID, "refresh_token": old.RefreshToken})
	if err != nil {
		return Tokens{}, Identity{}, err
	}
	t, i, err := o.exchange(ctx, body, "application/json", old, true)
	if err != nil {
		var httpErr *HTTPError
		if errors.As(err, &httpErr) {
			switch httpErr.Code {
			case "refresh_token_expired":
				return t, i, fmt.Errorf("刷新令牌已过期，请重新登录")
			case "refresh_token_reused":
				return t, i, fmt.Errorf("刷新令牌已被使用，请重新登录")
			case "refresh_token_invalidated", "invalid_grant":
				return t, i, fmt.Errorf("刷新令牌已失效，请重新登录")
			}
		}
	}
	return t, i, err
}

func (o OAuth) exchange(ctx context.Context, body []byte, contentType string, old Tokens, refresh bool) (Tokens, Identity, error) {
	var response tokenResponse
	raw, err := requestJSON(ctx, o.Client, http.MethodPost, o.Issuer+"/oauth/token", contentType, body, nil, &response)
	if err != nil {
		return Tokens{}, Identity{}, err
	}
	t := old
	if response.IDToken != "" {
		t.IDToken = response.IDToken
	}
	if response.AccessToken != "" {
		t.AccessToken = response.AccessToken
	}
	if response.RefreshToken != "" {
		t.RefreshToken = response.RefreshToken
	}
	if t.IDToken == "" || t.AccessToken == "" || t.RefreshToken == "" || (refresh && response.AccessToken == "") {
		return t, Identity{}, fmt.Errorf("OAuth 响应缺少必要令牌")
	}
	i, err := identityFromToken(t.IDToken)
	if err != nil {
		return t, i, err
	}
	access, err := tokenClaims(t.AccessToken)
	if err != nil {
		return t, i, err
	}
	accessIdentity, err := identityFromToken(t.AccessToken)
	if err != nil {
		return t, i, err
	}
	if accessIdentity.UserID != i.UserID || accessIdentity.AccountID != i.AccountID {
		return t, i, fmt.Errorf("访问令牌与身份令牌不匹配")
	}
	exp, err := access.GetExpirationTime()
	if err != nil {
		return t, i, err
	}
	if response.ExpiresIn > 0 {
		t.ExpiresAt = time.Now().Add(time.Duration(response.ExpiresIn) * time.Second)
	} else if exp != nil {
		t.ExpiresAt = exp.Time
	} else {
		return t, i, fmt.Errorf("OAuth 响应缺少令牌到期时间")
	}
	t.AccountID = i.AccountID
	t.LastRefresh = time.Now().UTC()
	t.OAuthResponse = raw
	return t, i, nil
}
