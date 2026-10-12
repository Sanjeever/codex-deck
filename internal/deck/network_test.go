package deck

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestUserOperationCancelsAutomaticBatchWithoutBlocking(t *testing.T) {
	s := testService(t)
	a := testAccount(t)
	addTestAccount(t, s, a)
	// 不需要第二账号：取消当前请求后必须立即返回，而非等待 HTTP 超时。
	entered := make(chan struct{})
	stopped := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(entered); <-r.Context().Done(); close(stopped) }))
	defer server.Close()
	s.backend = Backend{Client: server.Client(), BaseURL: server.URL}
	done := make(chan State, 1)
	go func() { done <- s.RefreshAutomatic() }()
	<-entered
	renamed := make(chan error, 1)
	go func() { renamed <- s.Rename(a.ID, "新别名") }()
	select {
	case err := <-renamed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("用户操作被网络请求阻塞")
	}
	select {
	case state := <-done:
		if state.Accounts[0].Alias != "新别名" || state.Accounts[0].UsageError != "" {
			t.Fatal("取消污染账号状态")
		}
	case <-time.After(time.Second):
		t.Fatal("自动请求未取消")
	}
	<-stopped
}

func TestQueryDoesNotOverwriteConcurrentAlias(t *testing.T) {
	s := testService(t)
	a := testAccount(t)
	addTestAccount(t, s, a)
	entered := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-release
		w.Write([]byte(`{"plan_type":"plus"}`))
	}))
	defer server.Close()
	s.backend = Backend{Client: server.Client(), BaseURL: server.URL}
	done := make(chan error, 1)
	go func() { done <- s.RefreshAccount(a.ID) }()
	<-entered
	if err := s.Rename(a.ID, "保留别名"); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if state := s.GetState(); state.Accounts[0].Alias != "保留别名" || state.Accounts[0].Usage == nil {
		t.Fatal("查询覆盖并发变更")
	}
}
