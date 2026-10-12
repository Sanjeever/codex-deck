package deck

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sync"
)

type Store struct {
	mu   sync.Mutex
	path string
	data Data
}

func OpenStore(path string) (*Store, error) {
	s := &Store{path: path}
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		s.data = Data{Version: Version, Accounts: []Account{}}
		b, err = json.MarshalIndent(s.data, "", "  ")
		if err == nil {
			err = atomicWrite(path, b)
		}
	} else if err == nil {
		s.data, err = parseData(b)
	}
	if err != nil {
		return nil, fmt.Errorf("读取配置失败: %w", err)
	}
	return s, nil
}

func validateSettings(s Settings) error {
	if s.WebDAVURL == "" {
		return nil
	}
	u, err := url.Parse(s.WebDAVURL)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("WebDAV 地址应为 HTTP 或 HTTPS 目录地址")
	}
	return nil
}

func parseData(b []byte) (Data, error) {
	var d Data
	if err := json.Unmarshal(b, &d); err != nil {
		return d, fmt.Errorf("JSON 格式无效: %w", err)
	}
	if d.Version != Version || d.Accounts == nil {
		return d, fmt.Errorf("不支持的配置格式或版本")
	}
	if err := validateSettings(d.Settings); err != nil {
		return d, err
	}
	ids := map[string]bool{}
	identities := map[Identity]bool{}
	for _, a := range d.Accounts {
		i, err := identityFromToken(a.Tokens.IDToken)
		if err != nil || a.ID == "" || !identityMatches(a, i) || a.Tokens.AccountID != a.AccountID || a.Tokens.AccessToken == "" || a.Tokens.RefreshToken == "" {
			return d, fmt.Errorf("账号记录或认证无效")
		}
		key := Identity{UserID: a.UserID, AccountID: a.AccountID}
		if ids[a.ID] || identities[key] {
			return d, fmt.Errorf("配置包含重复账号")
		}
		ids[a.ID], identities[key] = true, true
	}
	return d, nil
}

func cloneData(d Data) Data {
	b, err := json.Marshal(d)
	if err != nil {
		panic(err)
	}
	var result Data
	if err := json.Unmarshal(b, &result); err != nil {
		panic(err)
	}
	return result
}

func (s *Store) Snapshot() Data { s.mu.Lock(); defer s.mu.Unlock(); return cloneData(s.data) }

func (s *Store) Update(change func(*Data) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := cloneData(s.data)
	if err := change(&next); err != nil {
		return err
	}
	b, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return err
	}
	if _, err = parseData(b); err != nil {
		return err
	}
	if err = atomicWrite(s.path, b); err != nil {
		return fmt.Errorf("保存配置失败: %w", err)
	}
	s.data = next
	return nil
}

func atomicWrite(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".codex-deck-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), path)
}
