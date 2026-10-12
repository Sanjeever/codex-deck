package deck

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func waitCLIState(t *testing.T, s *Service, condition func(State) bool) {
	t.Helper()
	timeout := time.NewTimer(3 * time.Second)
	defer timeout.Stop()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		if condition(s.GetState()) {
			return
		}
		select {
		case <-timeout.C:
			t.Fatal("CLI 文件变化未同步")
		case <-tick.C:
		}
	}
}

func TestWatchAtomicLoginLogoutAndMalformedAuth(t *testing.T) {
	s := testService(t)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1); w.WriteHeader(500) }))
	defer server.Close()
	s.backend = Backend{Client: server.Client(), BaseURL: server.URL}
	if err := s.StartWatching(); err != nil {
		t.Fatal(err)
	}
	a := testAccount(t)
	if err := s.cli.Write(a.Tokens); err != nil {
		t.Fatal(err)
	}
	waitCLIState(t, s, func(state State) bool { return len(state.Accounts) == 1 && state.Accounts[0].Current })
	id := s.GetState().Accounts[0].ID
	if err := s.Delete(id); err != nil {
		t.Fatal(err)
	}
	// 应用写入相同身份不会重新加入本次会话删除的账号。
	if err := s.cli.Write(a.Tokens); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(s.cli.Home, "auth.json")); err != nil {
		t.Fatal(err)
	}
	a.UserID = "new-user"
	a.AccountID = "new-workspace"
	a.Tokens.AccountID = a.AccountID
	a.Tokens.IDToken = testToken(t, a.UserID, a.AccountID, time.Now().Add(time.Hour))
	a.Tokens.AccessToken = a.Tokens.IDToken
	if err := s.cli.Write(a.Tokens); err != nil {
		t.Fatal(err)
	}
	waitCLIState(t, s, func(state State) bool {
		return len(state.Accounts) == 1 && state.Accounts[0].AccountID == a.AccountID && state.Accounts[0].Current
	})
	if err := os.Remove(filepath.Join(s.cli.Home, "auth.json")); err != nil {
		t.Fatal(err)
	}
	waitCLIState(t, s, func(state State) bool { return !state.Accounts[0].Current })
	if err := os.WriteFile(filepath.Join(s.cli.Home, "auth.json"), []byte("invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	waitCLIState(t, s, func(state State) bool { return state.CLIError != "" })
	if err := s.cli.Write(a.Tokens); err != nil {
		t.Fatal(err)
	}
	waitCLIState(t, s, func(state State) bool { return state.CLIError == "" && state.Accounts[0].Current })
	if requests.Load() != 0 {
		t.Fatal("文件监听触发业务 HTTP 请求")
	}
}

func TestWatchCLIHomeCreatedAfterStartup(t *testing.T) {
	s := testService(t)
	s.cli.Home = filepath.Join(s.cli.Home, "later")
	if err := s.StartWatching(); err != nil {
		t.Fatal(err)
	}
	a := testAccount(t)
	if err := s.cli.Write(a.Tokens); err != nil {
		t.Fatal(err)
	}
	waitCLIState(t, s, func(state State) bool { return len(state.Accounts) == 1 && state.Accounts[0].Current })
	if err := os.Remove(filepath.Join(s.cli.Home, "auth.json")); err != nil {
		t.Fatal(err)
	}
	waitCLIState(t, s, func(state State) bool { return !state.Accounts[0].Current })
}
