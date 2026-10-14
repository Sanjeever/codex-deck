package deck

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCLICredentialSelectionAcrossEntryPoints(t *testing.T) {
	now := time.Now().UTC()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"plan_type":"plus"}`))
	}))
	defer server.Close()
	for _, tc := range []struct {
		name     string
		deckTime time.Time
		cliTime  time.Time
		same     bool
		useCLI   bool
		wantTime time.Time
	}{
		{"newer", now, now.Add(time.Minute), false, true, now.Add(time.Minute)},
		{"older", now, now.Add(-time.Minute), false, false, now},
		{"equal", now, now, false, false, now},
		{"missing_cli_time", now, time.Time{}, false, false, now},
		{"missing_deck_time", time.Time{}, now, false, false, time.Time{}},
		{"missing_both_times", time.Time{}, time.Time{}, false, false, time.Time{}},
		{"same_newer", now, now.Add(time.Minute), true, false, now.Add(time.Minute)},
		{"same_older", now, now.Add(-time.Minute), true, false, now},
		{"same_equal", now, now, true, false, now},
		{"same_missing_cli_time", now, time.Time{}, true, false, now},
		{"same_missing_deck_time", time.Time{}, now, true, false, now},
	} {
		for _, entry := range []string{"startup", "explicit", "automatic", "request", "export"} {
			t.Run(tc.name+"/"+entry, func(t *testing.T) {
				s := testService(t)
				account := testAccount(t)
				account.Alias = "保留别名"
				account.Tokens.LastRefresh = tc.deckTime
				account.Tokens.OAuthResponse = json.RawMessage(`{"scope":"openid"}`)
				addTestAccount(t, s, account)
				local := account.Tokens
				local.LastRefresh = tc.cliTime
				if !tc.same {
					local.RefreshToken = "cli-refresh"
				}
				if err := s.cli.Write(local); err != nil {
					t.Fatal(err)
				}
				var exported *Account
				s.backend = Backend{Client: server.Client(), BaseURL: server.URL}
				switch entry {
				case "startup":
					restarted, err := NewService(context.Background(), s.store.path, s.cli.Home)
					if err != nil {
						t.Fatal(err)
					}
					t.Cleanup(restarted.Shutdown)
					s = restarted
				case "explicit":
					s.RefreshAll()
				case "automatic":
					s.RefreshAutomatic()
				case "request":
					s.mu.Lock()
					_, err := s.prepareAccount(account.ID, false)
					s.mu.Unlock()
					if err != nil {
						t.Fatal(err)
					}
				case "export":
					path := filepath.Join(t.TempDir(), "auth.json")
					if err := s.ExportAuth(account.ID, path, false); err != nil {
						t.Fatal(err)
					}
					value, err := readAuthAccount(path)
					if err != nil {
						t.Fatal(err)
					}
					exported = &value
				}
				want := account.Tokens.RefreshToken
				if tc.useCLI {
					want = local.RefreshToken
				}
				data := s.store.Snapshot()
				stored := data.Accounts[0]
				if len(data.Accounts) != 1 || stored.ID != account.ID || stored.Alias != account.Alias || string(stored.Tokens.OAuthResponse) != string(account.Tokens.OAuthResponse) {
					t.Fatal("同步未保留原账号记录")
				}
				if stored.Tokens.RefreshToken != want || !stored.Tokens.LastRefresh.Equal(tc.wantTime) {
					t.Fatal("凭据选择或刷新时间错误")
				}
				if exported != nil && (exported.Tokens.RefreshToken != want || !exported.Tokens.LastRefresh.Equal(tc.wantTime)) {
					t.Fatal("导出与已保存凭据不一致")
				}
				cli, err := s.cli.ReadAccount()
				if err != nil {
					t.Fatal(err)
				}
				if cli.Tokens.RefreshToken != local.RefreshToken || !cli.Tokens.LastRefresh.Equal(local.LastRefresh) {
					t.Fatal("同步改写 CLI 凭据")
				}
			})
		}
	}
}

func TestCLITimestampChangeDoesNotClearAuthenticationFailure(t *testing.T) {
	s := testService(t)
	account := testAccount(t)
	account.AuthError = "请重新登录"
	addTestAccount(t, s, account)
	local := account.Tokens
	local.LastRefresh = local.LastRefresh.Add(time.Minute)
	if err := s.cli.Write(local); err != nil {
		t.Fatal(err)
	}
	s.RefreshAutomatic()
	stored := s.store.Snapshot().Accounts[0]
	if stored.AuthError != account.AuthError || !stored.Tokens.LastRefresh.Equal(local.LastRefresh) {
		t.Fatal("时间变化被误判为新凭据并清除认证失效")
	}
}

func TestRefreshDoesNotWriteBackDifferentCLICredentials(t *testing.T) {
	for _, missingTime := range []bool{false, true} {
		t.Run(map[bool]string{false: "equal_time", true: "missing_time"}[missingTime], func(t *testing.T) {
			s := testService(t)
			account := testAccount(t)
			addTestAccount(t, s, account)
			local := account.Tokens
			local.RefreshToken = "independent-cli-refresh"
			if missingTime {
				local.LastRefresh = time.Time{}
			}
			if err := s.cli.Write(local); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(s.cli.Home, "auth.json")
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			fresh := testToken(t, account.UserID, account.AccountID, time.Now().Add(2*time.Hour))
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]string
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if body["refresh_token"] != account.Tokens.RefreshToken {
					t.Error("未使用 Deck 已选择的凭据刷新")
				}
				json.NewEncoder(w).Encode(map[string]any{"id_token": fresh, "access_token": fresh, "refresh_token": "rotated-deck-refresh", "expires_in": 3600})
			}))
			defer server.Close()
			s.oauth = OAuth{Client: server.Client(), Issuer: server.URL}
			s.mu.Lock()
			_, err = s.prepareAccount(account.ID, true)
			s.mu.Unlock()
			if err != nil {
				t.Fatal(err)
			}
			if s.store.Snapshot().Accounts[0].Tokens.RefreshToken != "rotated-deck-refresh" {
				t.Fatal("刷新结果未保存到 Deck")
			}
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(after) != string(before) {
				t.Fatal("刷新覆盖同一身份的另一份 CLI 凭据")
			}
		})
	}
}
