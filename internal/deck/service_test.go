package deck

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func testService(t *testing.T) *Service {
	t.Helper()
	s, err := NewService(context.Background(), filepath.Join(t.TempDir(), "deck.json"), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Shutdown)
	return s
}

func addTestAccount(t *testing.T, s *Service, a Account) {
	t.Helper()
	if err := s.store.Update(func(d *Data) error { d.Accounts = append(d.Accounts, a); return nil }); err != nil {
		t.Fatal(err)
	}
}

func TestStartupImportsCLIWhenEmpty(t *testing.T) {
	for _, existingConfig := range []bool{false, true} {
		t.Run(fmt.Sprint(existingConfig), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "deck.json")
			if existingConfig {
				if _, err := OpenStore(path); err != nil {
					t.Fatal(err)
				}
			}
			cli := CLI{Home: t.TempDir()}
			config := []byte("# keep settings\ncli_auth_credentials_store = \"file\"\nmodel = \"test-model\"\n")
			if err := os.WriteFile(filepath.Join(cli.Home, "config.toml"), config, 0600); err != nil {
				t.Fatal(err)
			}
			a := testAccount(t)
			if err := cli.Write(a.Tokens); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(filepath.Join(cli.Home, "auth.json"))
			if err != nil {
				t.Fatal(err)
			}
			s, err := NewService(context.Background(), path, cli.Home)
			if err != nil {
				t.Fatal(err)
			}
			state := s.GetState()
			if len(state.Accounts) != 1 || !state.Accounts[0].Current || state.Accounts[0].Email != a.Email || state.Accounts[0].Plan != a.Plan || state.CLIError != "" {
				t.Fatal("启动未导入本地账号或当前状态错误")
			}
			reloaded, err := OpenStore(path)
			if err != nil {
				t.Fatal(err)
			}
			data := reloaded.Snapshot()
			imported := data.Accounts[0]
			if imported.ID == "" || imported.UserID != a.UserID || imported.AccountID != a.AccountID || imported.Tokens.IDToken != a.Tokens.IDToken || imported.Tokens.AccessToken != a.Tokens.AccessToken || imported.Tokens.RefreshToken != a.Tokens.RefreshToken || !imported.Tokens.LastRefresh.Equal(a.Tokens.LastRefresh) || imported.Tokens.ExpiresAt.IsZero() || data.LastSwitchedID != "" {
				t.Fatal("导入丢失身份或凭据")
			}
			second, err := NewService(context.Background(), path, cli.Home)
			if err != nil {
				t.Fatal(err)
			}
			if accounts := second.GetState().Accounts; len(accounts) != 1 || accounts[0].ID != imported.ID {
				t.Fatal("重启生成重复账号")
			}
			if err := second.Delete(imported.ID); err == nil {
				t.Fatal("删除当前账号未被拒绝")
			}
			if len(second.GetState().Accounts) != 1 {
				t.Fatal("拒绝删除后账号丢失")
			}
			after, err := os.ReadFile(filepath.Join(cli.Home, "auth.json"))
			if err != nil {
				t.Fatal(err)
			}
			configAfter, err := os.ReadFile(filepath.Join(cli.Home, "config.toml"))
			if err != nil {
				t.Fatal(err)
			}
			if string(after) != string(before) || string(configAfter) != string(config) {
				t.Fatal("导入或删除改写 CLI 文件")
			}
		})
	}
}

func TestStartupImportsCLIAlongsideExistingAccounts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "deck.json")
	store, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	a := testAccount(t)
	a.Alias = "已保存账号"
	if err := store.Update(func(d *Data) error { d.Accounts = []Account{a}; return nil }); err != nil {
		t.Fatal(err)
	}
	cli := CLI{Home: t.TempDir()}
	other := a.Tokens
	other.AccountID = "other-workspace"
	other.IDToken = testToken(t, a.UserID, other.AccountID, time.Now().Add(time.Hour))
	other.AccessToken = other.IDToken
	if err := cli.Write(other); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(cli.Home, "auth.json"))
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewService(context.Background(), path, cli.Home)
	if err != nil {
		t.Fatal(err)
	}
	state := s.GetState()
	if len(state.Accounts) != 2 || state.Accounts[0].ID != a.ID || state.Accounts[0].Alias != a.Alias || state.Accounts[0].Current || !state.Accounts[1].Current || state.Accounts[1].AccountID != other.AccountID {
		t.Fatal("启动未同步新账号或覆盖已有账号")
	}
	after, err := os.ReadFile(filepath.Join(cli.Home, "auth.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("同步改写 CLI 认证")
	}
}

func TestStartupImportMissingOrInvalidCredentials(t *testing.T) {
	a := testAccount(t)
	valid := cliAuth{AuthMode: "chatgpt", Tokens: &cliTokens{IDToken: a.Tokens.IDToken, AccessToken: a.Tokens.AccessToken, RefreshToken: a.Tokens.RefreshToken, AccountID: a.AccountID}, LastRefresh: &a.Tokens.LastRefresh}
	encode := func(auth cliAuth) []byte {
		b, err := json.Marshal(auth)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	incomplete := valid
	incomplete.Tokens = &cliTokens{IDToken: a.Tokens.IDToken, AccessToken: a.Tokens.AccessToken, AccountID: a.AccountID}
	mismatch := valid
	mismatch.Tokens = &cliTokens{IDToken: a.Tokens.IDToken, AccessToken: testToken(t, "other-user", a.AccountID, time.Now().Add(time.Hour)), RefreshToken: a.Tokens.RefreshToken, AccountID: a.AccountID}
	for _, tc := range []struct {
		name      string
		auth      []byte
		mode      string
		wantError bool
	}{
		{name: "missing"},
		{name: "invalid_json", auth: []byte(`{"tokens":`), wantError: true},
		{name: "incomplete", auth: encode(incomplete), wantError: true},
		{name: "identity_mismatch", auth: encode(mismatch), wantError: true},
		{name: "keyring", auth: encode(valid), mode: "keyring", wantError: true},
		{name: "api_key", auth: []byte(`{"auth_mode":"apikey","OPENAI_API_KEY":"local-test-only"}`), wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cli := CLI{Home: t.TempDir()}
			if tc.auth != nil {
				if err := os.WriteFile(filepath.Join(cli.Home, "auth.json"), tc.auth, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if tc.mode != "" {
				if err := os.WriteFile(filepath.Join(cli.Home, "config.toml"), []byte(fmt.Sprintf("cli_auth_credentials_store = %q\n", tc.mode)), 0600); err != nil {
					t.Fatal(err)
				}
			}
			path := filepath.Join(t.TempDir(), "deck.json")
			s, err := NewService(context.Background(), path, cli.Home)
			if err != nil {
				t.Fatal("导入失败不应阻止应用启动", err)
			}
			state := s.GetState()
			if len(state.Accounts) != 0 || (state.CLIError != "") != tc.wantError {
				t.Fatal("无效认证被导入或未显示错误")
			}
			if len(s.store.Snapshot().Accounts) != 0 {
				t.Fatal("无效凭据被保存")
			}
			if tc.auth != nil {
				b, err := os.ReadFile(filepath.Join(cli.Home, "auth.json"))
				if err != nil {
					t.Fatal(err)
				}
				if string(b) != string(tc.auth) {
					t.Fatal("改写无效 CLI 认证")
				}
			}
		})
	}
}

func TestStartupImportBacksUpAccount(t *testing.T) {
	settings, _, closeServer := testWebDAV(t)
	defer closeServer()
	path := filepath.Join(t.TempDir(), "deck.json")
	store, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Update(func(d *Data) error { d.Settings = settings; return nil }); err != nil {
		t.Fatal(err)
	}
	cli := CLI{Home: t.TempDir()}
	a := testAccount(t)
	if err := cli.Write(a.Tokens); err != nil {
		t.Fatal(err)
	}
	s, err := NewService(context.Background(), path, cli.Home)
	if err != nil {
		t.Fatal(err)
	}
	waitBackup(t, s)
	remote, err := downloadBackup(settings)
	if err != nil {
		t.Fatal(err)
	}
	if len(remote.Accounts) != 1 || remote.Accounts[0].Tokens.RefreshToken != a.Tokens.RefreshToken || s.GetState().Backup.At == nil || s.GetState().Backup.Error != "" {
		t.Fatal("自动导入未触发备份")
	}
}

func TestRefreshSynchronizesManualCLILoginsAndLogout(t *testing.T) {
	s := testService(t)
	a := testAccount(t)
	a.Alias = "原账号"
	addTestAccount(t, s, a)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Write([]byte(`{"plan_type":"plus"}`))
	}))
	defer server.Close()
	s.backend = Backend{Client: server.Client(), BaseURL: server.URL}
	b := testAccount(t)
	b.UserID = "other-user"
	b.AccountID = "other-workspace"
	b.Tokens.IDToken = testToken(t, b.UserID, b.AccountID, time.Now().Add(time.Hour))
	b.Tokens.AccessToken = b.Tokens.IDToken
	b.Tokens.AccountID = b.AccountID
	if err := s.cli.Write(b.Tokens); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(s.cli.Home, "auth.json"))
	if err != nil {
		t.Fatal(err)
	}
	state := s.RefreshAll()
	if len(state.Accounts) != 2 || state.Accounts[0].Current || !state.Accounts[1].Current || state.Accounts[0].Alias != a.Alias || state.Accounts[1].AccountID != b.AccountID || requests.Load() != 2 {
		t.Fatal("刷新未发现本地新账号或未查询全部账号")
	}
	newID := state.Accounts[1].ID
	after, err := os.ReadFile(filepath.Join(s.cli.Home, "auth.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("同步改写 CLI 凭据")
	}
	if err := os.Remove(filepath.Join(s.cli.Home, "auth.json")); err != nil {
		t.Fatal(err)
	}
	state = s.RefreshAll()
	if len(state.Accounts) != 2 || state.Accounts[0].Current || state.Accounts[1].Current || state.CLIError != "" {
		t.Fatal("logout 后当前标记或账号保留错误")
	}
	if _, err := os.Stat(filepath.Join(s.cli.Home, "auth.json")); !os.IsNotExist(err) {
		t.Fatal("logout 后重新写入 CLI 认证")
	}
	if err := s.cli.Write(a.Tokens); err != nil {
		t.Fatal(err)
	}
	if err := s.RefreshAccount(a.ID); err != nil {
		t.Fatal(err)
	}
	state = s.GetState()
	if len(state.Accounts) != 2 || !state.Accounts[0].Current || state.Accounts[1].Current || state.Accounts[1].ID != newID {
		t.Fatal("行内刷新未同步当前账号或生成重复记录")
	}
	reloaded, err := OpenStore(s.store.path)
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded.Snapshot().Accounts) != 2 {
		t.Fatal("发现的新账号未持久保存")
	}
}

func TestRefreshUpdatesCLICredentialsPreservingAccountData(t *testing.T) {
	s := testService(t)
	a := testAccount(t)
	a.Alias = "保留别名"
	a.Usage = &Usage{Plan: "plus", FiveHour: &Window{RemainingPercent: 42}, QueriedAt: time.Now()}
	a.Credits = &Credits{AvailableCount: 1, QueriedAt: time.Now()}
	a.PendingReset = &ResetRequest{ID: "pending-local-request", Started: true}
	a.Tokens.OAuthResponse = json.RawMessage(`{"scope":"openid"}`)
	addTestAccount(t, s, a)
	updated := a.Tokens
	updated.RefreshToken = "manual-login-refresh"
	updated.LastRefresh = a.Tokens.LastRefresh.Add(time.Second)
	if err := s.cli.Write(updated); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusForbidden) }))
	defer server.Close()
	s.backend = Backend{Client: server.Client(), BaseURL: server.URL}
	state := s.RefreshAll()
	stored := s.store.Snapshot().Accounts[0]
	if len(state.Accounts) != 1 || stored.ID != a.ID || stored.Alias != a.Alias || stored.Tokens.RefreshToken != updated.RefreshToken || !stored.Tokens.LastRefresh.Equal(updated.LastRefresh) || string(stored.Tokens.OAuthResponse) != string(a.Tokens.OAuthResponse) || stored.Usage.FiveHour.RemainingPercent != 42 || stored.Credits.AvailableCount != 1 || stored.PendingReset.ID != a.PendingReset.ID || state.Accounts[0].UsageError == "" {
		t.Fatal("凭据更新丢失账号数据或刷新失败覆盖缓存")
	}
}

func TestRefreshSyncFailureAndRecovery(t *testing.T) {
	s := testService(t)
	a := testAccount(t)
	addTestAccount(t, s, a)
	invalid := cliAuth{AuthMode: "chatgpt", Tokens: &cliTokens{IDToken: a.Tokens.IDToken, AccessToken: a.Tokens.AccessToken, AccountID: a.AccountID}}
	b, err := json.Marshal(invalid)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.cli.Home, "auth.json"), b, 0600); err != nil {
		t.Fatal(err)
	}
	state := s.RefreshAll()
	if state.CLIError == "" || len(state.Accounts) != 1 || s.store.Snapshot().Accounts[0].Tokens.RefreshToken != a.Tokens.RefreshToken {
		t.Fatal("同步失败未报错或覆盖已有认证")
	}
	if err := s.cli.Write(a.Tokens); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"plan_type":"plus"}`)) }))
	defer server.Close()
	s.backend = Backend{Client: server.Client(), BaseURL: server.URL}
	state = s.RefreshAll()
	if state.CLIError != "" || len(state.Accounts) != 1 || !state.Accounts[0].Current {
		t.Fatal("修复凭据后同步错误未清除")
	}
}

func TestCLISyncBacksUpOnlyChangedCredentials(t *testing.T) {
	settings, root, closeServer := testWebDAV(t)
	defer closeServer()
	s := testService(t)
	a := testAccount(t)
	addTestAccount(t, s, a)
	if err := s.cli.Write(a.Tokens); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveSettings(settings); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"plan_type":"plus"}`)) }))
	defer server.Close()
	s.backend = Backend{Client: server.Client(), BaseURL: server.URL}
	waitBackup(t, s)
	before, err := os.ReadFile(filepath.Join(root, "backup", "codex-deck.json"))
	if err != nil {
		t.Fatal(err)
	}
	backupAt := *s.GetState().Backup.At
	s.RefreshAll()
	after, err := os.ReadFile(filepath.Join(root, "backup", "codex-deck.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) || !s.GetState().Backup.At.Equal(backupAt) {
		t.Fatal("相同凭据反复触发备份")
	}
	updated := a.Tokens
	updated.RefreshToken = "cli-new-refresh"
	updated.LastRefresh = a.Tokens.LastRefresh.Add(time.Second)
	if err := s.cli.Write(updated); err != nil {
		t.Fatal(err)
	}
	s.RefreshAll()
	waitBackup(t, s)
	remote, err := downloadBackup(settings)
	if err != nil {
		t.Fatal(err)
	}
	if remote.Accounts[0].Tokens.RefreshToken != updated.RefreshToken {
		t.Fatal("CLI 更新凭据后未备份")
	}
	backupAt = *s.GetState().Backup.At
	s.RefreshAll()
	if !s.GetState().Backup.At.Equal(backupAt) {
		t.Fatal("同步完成后再次触发相同备份")
	}
}

func TestCLISyncKeepsNewerDeckLogin(t *testing.T) {
	s := testService(t)
	a := testAccount(t)
	if err := s.cli.Write(a.Tokens); err != nil {
		t.Fatal(err)
	}
	a.Tokens.RefreshToken = "newer-deck-login"
	a.Tokens.LastRefresh = a.Tokens.LastRefresh.Add(time.Second)
	addTestAccount(t, s, a)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"plan_type":"plus"}`)) }))
	defer server.Close()
	s.backend = Backend{Client: server.Client(), BaseURL: server.URL}
	s.RefreshAll()
	if s.store.Snapshot().Accounts[0].Tokens.RefreshToken != "newer-deck-login" {
		t.Fatal("CLI 旧凭据覆盖 Deck 新登录")
	}
}

func TestCLISyncUnsupportedStoreAllowsConfirmedSwitch(t *testing.T) {
	s := testService(t)
	a := testAccount(t)
	addTestAccount(t, s, a)
	if err := os.WriteFile(filepath.Join(s.cli.Home, "config.toml"), []byte("cli_auth_credentials_store = \"keyring\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	restarted, err := NewService(context.Background(), s.store.path, s.cli.Home)
	if err != nil {
		t.Fatal(err)
	}
	if state := restarted.GetState(); state.CLIError == "" || state.CLIStore != "keyring" {
		t.Fatal("不支持的存储方式未显示同步错误")
	}
	if err := restarted.Switch(a.ID, false); err == nil {
		t.Fatal("未确认便修改凭据存储")
	}
	if err := restarted.Switch(a.ID, true); err != nil {
		t.Fatal(err)
	}
	if state := restarted.GetState(); state.CLIError != "" || state.CLIStore != "file" || !state.Accounts[0].Current {
		t.Fatal("确认切换后同步错误未清除")
	}
}

func TestAccountLoginDedupIdentityAndDelete(t *testing.T) {
	s := testService(t)
	a := testAccount(t)
	i, err := identityFromToken(a.Tokens.IDToken)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.acceptLogin("", a.Tokens, i); err != nil {
		t.Fatal(err)
	}
	id := s.store.Snapshot().Accounts[0].ID
	a.Tokens.RefreshToken = "updated"
	if err = s.acceptLogin("", a.Tokens, i); err != nil {
		t.Fatal(err)
	}
	if len(s.store.Snapshot().Accounts) != 1 || s.store.Snapshot().Accounts[0].Tokens.RefreshToken != "updated" {
		t.Fatal("登录生成重复记录或未更新认证")
	}
	other := i
	other.AccountID = "other-workspace"
	if err = s.acceptLogin(id, a.Tokens, other); err == nil {
		t.Fatal("重登接受不同身份")
	}
	if err = s.Switch(id, false); err != nil {
		t.Fatal(err)
	}
	if !s.GetState().Accounts[0].Current {
		t.Fatal("当前账号识别错误")
	}
	if err = s.Rename(id, "新别名"); err != nil {
		t.Fatal(err)
	}
	if err = s.Delete(id); err == nil {
		t.Fatal("删除当前账号未被拒绝")
	}
	if len(s.GetState().Accounts) != 1 {
		t.Fatal("拒绝删除后账号丢失")
	}
	_, cliIdentity, err := s.cli.Read()
	if err != nil {
		t.Fatal(err)
	}
	if cliIdentity.AccountID != i.AccountID {
		t.Fatal("删除修改 CLI")
	}
}

func TestRefreshFailurePreservesCacheAndContinues(t *testing.T) {
	s := testService(t)
	a := testAccount(t)
	a.Usage = &Usage{Plan: "plus", FiveHour: &Window{RemainingPercent: 42}, QueriedAt: time.Now()}
	addTestAccount(t, s, a)
	other := a
	other.ID = "other-record"
	other.UserID = "other-user"
	other.Tokens.IDToken = testToken(t, other.UserID, other.AccountID, time.Now().Add(time.Hour))
	other.Tokens.AccessToken = other.Tokens.IDToken
	other.Usage = nil
	addTestAccount(t, s, other)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "Bearer "+a.Tokens.AccessToken {
			w.WriteHeader(403)
			return
		}
		w.Write([]byte(`{"plan_type":"pro","rate_limit":{"primary_window":{"limit_window_seconds":18000,"used_percent":10}}}`))
	}))
	defer server.Close()
	s.backend = Backend{Client: server.Client(), BaseURL: server.URL}
	state := s.RefreshAll()
	if state.Accounts[0].Usage.FiveHour.RemainingPercent != 42 || state.Accounts[0].UsageError == "" {
		t.Fatal("失败覆盖缓存或隐藏错误")
	}
	if state.Accounts[1].Usage.FiveHour.RemainingPercent != 90 || state.Accounts[1].Plan != "pro" {
		t.Fatal("单账号失败阻止其他查询")
	}
}

func TestRefreshSavesTokensAndDetectsCLIChange(t *testing.T) {
	for _, concurrent := range []bool{false, true} {
		t.Run(fmt.Sprint(concurrent), func(t *testing.T) {
			s := testService(t)
			a := testAccount(t)
			a.Tokens.ExpiresAt = time.Now().Add(-time.Hour)
			addTestAccount(t, s, a)
			expired := testToken(t, a.UserID, a.AccountID, time.Now().Add(-time.Hour))
			a.Tokens.IDToken = expired
			a.Tokens.AccessToken = expired
			a.Tokens.LastRefresh = time.Now()
			if err := s.saveAccount(a); err != nil {
				t.Fatal(err)
			}
			if err := s.cli.Write(a.Tokens); err != nil {
				t.Fatal(err)
			}
			fresh := testToken(t, a.UserID, a.AccountID, time.Now().Add(time.Hour))
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]string
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if body["refresh_token"] != a.Tokens.RefreshToken {
					t.Error("使用过时副本")
				}
				if concurrent {
					c := a
					c.Tokens.RefreshToken = "cli-concurrent"
					if err := s.cli.Write(c.Tokens); err != nil {
						t.Error(err)
					}
				}
				json.NewEncoder(w).Encode(map[string]interface{}{"id_token": fresh, "access_token": fresh, "refresh_token": "new-refresh", "expires_in": 3600})
			}))
			defer server.Close()
			s.oauth = OAuth{Client: server.Client(), Issuer: server.URL}
			s.mu.Lock()
			_, err := s.prepareAccount(a.ID, false)
			s.mu.Unlock()
			if (err != nil) != concurrent {
				t.Fatalf("CLI 并发变化处理错误: %v", err)
			}
			if s.store.Snapshot().Accounts[0].Tokens.RefreshToken != "new-refresh" {
				t.Fatal("刷新未持久保存")
			}
			auth, _, err := s.cli.Read()
			if err != nil {
				t.Fatal(err)
			}
			want := "new-refresh"
			if concurrent {
				want = "cli-concurrent"
			}
			if auth.Tokens.RefreshToken != want {
				t.Fatal("CLI 写回错误")
			}
		})
	}
}

func TestAutoBackupOrderingFailuresAndUsageExclusion(t *testing.T) {
	s := testService(t)
	a := testAccount(t)
	addTestAccount(t, s, a)
	settings, root, closeServer := testWebDAV(t)
	defer closeServer()
	if err := s.SaveSettings(settings); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for n := 0; n < 5; n++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			if err := s.Rename(a.ID, fmt.Sprint(n)); err != nil {
				t.Error(err)
			}
		}(n)
	}
	wg.Wait()
	if err := s.Backup(); err != nil {
		t.Fatal(err)
	}
	waitBackup(t, s)
	remote, err := downloadBackup(settings)
	if err != nil {
		t.Fatal(err)
	}
	if remote.Accounts[0].Alias != s.store.Snapshot().Accounts[0].Alias {
		t.Fatal("旧备份覆盖新数据")
	}
	waitBackup(t, s)
	before, err := os.ReadFile(filepath.Join(root, "backup", "codex-deck.json"))
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"plan_type":"plus"}`)) }))
	defer server.Close()
	s.backend = Backend{Client: server.Client(), BaseURL: server.URL}
	s.RefreshAll()
	after, err := os.ReadFile(filepath.Join(root, "backup", "codex-deck.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("单纯用量刷新触发备份")
	}
	settings.WebDAVPassword = "wrong"
	if err = s.SaveSettings(settings); err != nil {
		t.Fatal(err)
	}
	if err = s.Backup(); err == nil {
		t.Fatal("错误凭据未暴露")
	}
	if s.GetState().Backup.Error == "" || s.store.Snapshot().Settings.WebDAVPassword != "wrong" {
		t.Fatal("备份失败隐藏或撤销本地保存")
	}
}

func TestRestoreConfirmationAndCLIUntouched(t *testing.T) {
	s := testService(t)
	a := testAccount(t)
	addTestAccount(t, s, a)
	if err := s.Switch(a.ID, false); err != nil {
		t.Fatal(err)
	}
	settings, _, closeServer := testWebDAV(t)
	defer closeServer()
	if err := s.SaveSettings(settings); err != nil {
		t.Fatal(err)
	}
	if err := s.Backup(); err != nil {
		t.Fatal(err)
	}
	remote := Data{Version: Version, Settings: settings, Accounts: []Account{}}
	if err := uploadBackup(remote); err != nil {
		t.Fatal(err)
	}
	cliBefore, err := os.ReadFile(filepath.Join(s.cli.Home, "auth.json"))
	if err != nil {
		t.Fatal(err)
	}
	preview, err := s.PrepareRestore()
	if err != nil {
		t.Fatal(err)
	}
	if len(s.store.Snapshot().Accounts) != 1 {
		t.Fatal("下载即覆盖本地")
	}
	s.CancelRestore()
	if err := s.Restore(preview.ID); err == nil {
		t.Fatal("接受取消预览")
	}
	preview, err = s.PrepareRestore()
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Restore(preview.ID); err != nil {
		t.Fatal(err)
	}
	if len(s.GetState().Accounts) != 0 {
		t.Fatal("恢复未整体替换")
	}
	cliAfter, err := os.ReadFile(filepath.Join(s.cli.Home, "auth.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(cliBefore) != string(cliAfter) {
		t.Fatal("恢复切换 CLI")
	}
	downloaded, err := downloadBackup(settings)
	if err != nil {
		t.Fatal(err)
	}
	if len(downloaded.Accounts) != 0 {
		t.Fatal("恢复自动上传")
	}
}

func TestConsumePersistentIdempotencyAndRefresh(t *testing.T) {
	s := testService(t)
	a := testAccount(t)
	addTestAccount(t, s, a)
	var attempts atomic.Int32
	var gotID string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/wham/rate-limit-reset-credits/consume":
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if gotID != "" && body["redeem_request_id"] != gotID {
				t.Error("重试更换请求标识")
			}
			gotID = body["redeem_request_id"]
			if attempts.Add(1) == 1 {
				w.WriteHeader(503)
				return
			}
			w.Write([]byte(`{"code":"reset","windows_reset":2}`))
		case "/wham/usage":
			w.Write([]byte(`{"plan_type":"plus","rate_limit":{"primary_window":{"limit_window_seconds":18000,"used_percent":5}},"rate_limit_reset_credits":{"available_count":0}}`))
		case "/wham/rate-limit-reset-credits":
			w.Write([]byte(`{"available_count":0,"credits":[]}`))
		default:
			t.Error("错误地址")
		}
	}))
	defer server.Close()
	s.backend = Backend{Client: server.Client(), BaseURL: server.URL}
	req, err := s.PrepareConsume(a.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Consume(a.ID, req.ID); err == nil {
		t.Fatal("未暴露消费失败")
	}
	reloaded, err := OpenStore(s.store.path)
	if err != nil {
		t.Fatal(err)
	}
	s.store = reloaded
	retry, err := s.PrepareConsume(a.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if retry.ID != req.ID {
		t.Fatal("重启丢失请求标识")
	}
	result, err := s.Consume(a.ID, retry.ID)
	if err != nil {
		t.Fatal(err)
	}
	if result.Code != "reset" {
		t.Fatal("消费结果错误")
	}
	if _, err = s.Consume(a.ID, retry.ID); err != nil {
		t.Fatal(err)
	}
	if attempts.Load() != 2 {
		t.Fatal("重复点击重复消费")
	}
	state := s.GetState()
	if state.Accounts[0].Usage.FiveHour.RemainingPercent != 95 || state.Accounts[0].Credits.AvailableCount != 0 {
		t.Fatal("消费后未查询或伪造额度")
	}
}

func TestUnauthorizedRefreshAndChangedIdentity(t *testing.T) {
	for _, changed := range []bool{false, true} {
		t.Run(fmt.Sprint(changed), func(t *testing.T) {
			s := testService(t)
			a := testAccount(t)
			addTestAccount(t, s, a)
			user := a.UserID
			if changed {
				user = "other-user"
			}
			fresh := testToken(t, user, a.AccountID, time.Now().Add(2*time.Hour))
			var requests, refreshes atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/oauth/token":
					refreshes.Add(1)
					json.NewEncoder(w).Encode(map[string]interface{}{"id_token": fresh, "access_token": fresh, "refresh_token": "refreshed", "expires_in": 3600})
				case "/wham/usage":
					if requests.Add(1) == 1 {
						w.WriteHeader(401)
						return
					}
					if r.Header.Get("Authorization") != "Bearer "+fresh {
						t.Error("未使用新访问令牌")
					}
					w.Write([]byte(`{"plan_type":"plus"}`))
				default:
					t.Error("错误请求路径")
				}
			}))
			defer server.Close()
			s.backend = Backend{Client: server.Client(), BaseURL: server.URL}
			s.oauth = OAuth{Client: server.Client(), Issuer: server.URL}
			err := s.RefreshAccount(a.ID)
			if (err != nil) != changed {
				t.Fatalf("身份变化处理错误: %v", err)
			}
			if refreshes.Load() != 1 {
				t.Fatal("令牌刷新未限制为一次")
			}
			wantRequests := int32(2)
			wantRefresh := "refreshed"
			if changed {
				wantRequests = 1
				wantRefresh = a.Tokens.RefreshToken
			}
			if requests.Load() != wantRequests || s.store.Snapshot().Accounts[0].Tokens.RefreshToken != wantRefresh {
				t.Fatal("刷新覆盖不同账号认证或错误重试")
			}
		})
	}
}

func TestSameEmailDifferentWorkspacesRemainDistinct(t *testing.T) {
	s := testService(t)
	for _, workspace := range []string{"workspace-1", "workspace-2"} {
		a := testAccount(t)
		a.AccountID = workspace
		a.Tokens.AccountID = workspace
		a.Tokens.IDToken = testToken(t, a.UserID, workspace, time.Now().Add(time.Hour))
		a.Tokens.AccessToken = a.Tokens.IDToken
		i, err := identityFromToken(a.Tokens.IDToken)
		if err != nil {
			t.Fatal(err)
		}
		if err = s.acceptLogin("", a.Tokens, i); err != nil {
			t.Fatal(err)
		}
	}
	accounts := s.store.Snapshot().Accounts
	if len(accounts) != 2 || accounts[0].ID == accounts[1].ID {
		t.Fatal("相同邮箱的不同工作区被合并")
	}
	if err := s.cli.Write(accounts[1].Tokens); err != nil {
		t.Fatal(err)
	}
	state := s.GetState()
	if state.Accounts[0].Current || !state.Accounts[1].Current {
		t.Fatal("当前账号未核对工作区")
	}
	if err := s.Delete(accounts[1].ID); err == nil {
		t.Fatal("未保护当前工作区账号")
	}
	if err := s.Delete(accounts[0].ID); err != nil {
		t.Fatal(err)
	}
	if len(s.GetState().Accounts) != 1 || !s.GetState().Accounts[0].Current {
		t.Fatal("删除其他工作区后当前账号丢失")
	}
}

func TestDeleteChecksLatestCLIIdentity(t *testing.T) {
	s := testService(t)
	a := testAccount(t)
	b := testAccount(t)
	b.ID = "other-account"
	b.AccountID = "other-workspace"
	b.Tokens.AccountID = b.AccountID
	b.Tokens.IDToken = testToken(t, b.UserID, b.AccountID, time.Now().Add(time.Hour))
	b.Tokens.AccessToken = b.Tokens.IDToken
	addTestAccount(t, s, a)
	addTestAccount(t, s, b)
	if err := s.Switch(a.ID, false); err != nil {
		t.Fatal(err)
	}
	if err := s.cli.Write(b.Tokens); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(s.cli.Home, "auth.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(b.ID); err == nil || err.Error() != "不能删除当前账号，请先切换到其他账号" {
		t.Fatalf("未按 CLI 最新身份拒绝删除: %v", err)
	}
	if err := s.Delete(a.ID); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(filepath.Join(s.cli.Home, "auth.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) || len(s.GetState().Accounts) != 1 || s.GetState().Accounts[0].ID != b.ID {
		t.Fatal("删除非当前账号改写 CLI 或丢失当前账号")
	}
}

func TestDeleteRejectsUnreadableCurrentIdentity(t *testing.T) {
	s := testService(t)
	a := testAccount(t)
	addTestAccount(t, s, a)
	if err := os.WriteFile(filepath.Join(s.cli.Home, "auth.json"), []byte("invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(a.ID); err == nil {
		t.Fatal("无法核对 CLI 身份时仍删除账号")
	}
	if len(s.store.Snapshot().Accounts) != 1 {
		t.Fatal("核对失败改写账号数据")
	}
}
