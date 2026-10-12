package deck

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestPermanentAuthenticationFailurePausesUntilNewLogin(t *testing.T) {
	s := testService(t)
	a := testAccount(t)
	a.Tokens.ExpiresAt = time.Now().Add(-time.Minute)
	addTestAccount(t, s, a)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(401)
		w.Write([]byte(`{"error":{"code":"refresh_token_reused"}}`))
	}))
	defer server.Close()
	s.oauth = OAuth{Client: server.Client(), Issuer: server.URL}
	state := s.RefreshAutomatic()
	if calls.Load() != 1 || state.Accounts[0].AuthError == "" {
		t.Fatal("永久失效未显示重新登录状态")
	}
	s.RefreshAutomatic()
	s.RefreshAll()
	if calls.Load() != 1 {
		t.Fatal("失效令牌重复请求")
	}
	reloaded, err := NewService(context.Background(), s.store.path, s.cli.Home)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.GetState().Accounts[0].AuthError == "" {
		t.Fatal("重启丢失认证状态")
	}
	s.mu.Lock()
	err = s.acceptLogin(a.ID, a.Tokens, Identity{UserID: a.UserID, AccountID: a.AccountID, Email: a.Email, Plan: a.Plan})
	s.mu.Unlock()
	if err != nil || s.GetState().Accounts[0].AuthError != "" {
		t.Fatal("新登录未清除认证失效")
	}
}

func TestTemporaryAuthenticationFailureDoesNotDisableAccount(t *testing.T) {
	s := testService(t)
	a := testAccount(t)
	a.Tokens.ExpiresAt = time.Now().Add(-time.Minute)
	addTestAccount(t, s, a)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) }))
	defer server.Close()
	s.oauth = OAuth{Client: server.Client(), Issuer: server.URL}
	state := s.RefreshAutomatic()
	if state.Accounts[0].AuthError != "" || state.Accounts[0].UsageError == "" {
		t.Fatal("临时故障被误判为认证失效")
	}
}
