package deck

import (
	"errors"
	"fmt"
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

func TestRestoreDiscardsInFlightUsageAndCreditResults(t *testing.T) {
	for _, credits := range []bool{false, true} {
		t.Run(fmt.Sprint(credits), func(t *testing.T) {
			s := testService(t)
			a := testAccount(t)
			addTestAccount(t, s, a)
			settings, _, closeDAV := testWebDAV(t)
			defer closeDAV()
			if err := s.SaveSettings(settings); err != nil {
				t.Fatal(err)
			}
			waitBackup(t, s)
			remote := s.store.Snapshot()
			remote.Accounts[0].Alias = "恢复后的别名"
			if err := uploadBackup(remote); err != nil {
				t.Fatal(err)
			}
			preview, err := prepareRestoreTest(t, s)
			if err != nil {
				t.Fatal(err)
			}
			entered := make(chan struct{})
			release := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				close(entered)
				<-release
				if credits {
					w.Write([]byte(`{"available_count":1,"credits":[]}`))
				} else {
					w.Write([]byte(`{"plan_type":"plus"}`))
				}
			}))
			defer server.Close()
			s.backend = Backend{Client: server.Client(), BaseURL: server.URL}
			done := make(chan error, 1)
			go func() {
				if credits {
					done <- s.RefreshCredits(a.ID)
				} else {
					done <- s.RefreshAccount(a.ID)
				}
			}()
			<-entered
			if err := s.Restore(preview.ID); err != nil {
				t.Fatal(err)
			}
			close(release)
			if err := <-done; !errors.Is(err, ErrAccountChanged) {
				t.Fatalf("恢复前请求仍写入: %v", err)
			}
			current := s.GetState().Accounts[0]
			if current.Alias != "恢复后的别名" || current.Usage != nil || current.Credits != nil || current.UsageError != "" || current.CreditsError != "" {
				t.Fatal("恢复数据被过期请求修改")
			}
		})
	}
}

func TestRestoreDuringConsumeDoesNotAttachResultToRestoredAccount(t *testing.T) {
	s := testService(t)
	a := testAccount(t)
	addTestAccount(t, s, a)
	settings, _, closeDAV := testWebDAV(t)
	defer closeDAV()
	if err := s.SaveSettings(settings); err != nil {
		t.Fatal(err)
	}
	waitBackup(t, s)
	request, err := s.PrepareConsume(a.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	remote := s.store.Snapshot()
	remote.Accounts[0].PendingReset = nil
	if err := uploadBackup(remote); err != nil {
		t.Fatal(err)
	}
	preview, err := prepareRestoreTest(t, s)
	if err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-release
		w.Write([]byte(`{"code":"reset","windows_reset":1}`))
	}))
	defer server.Close()
	s.backend = Backend{Client: server.Client(), BaseURL: server.URL}
	type outcome struct {
		result ConsumeResult
		err    error
	}
	done := make(chan outcome, 1)
	go func() { result, err := s.Consume(a.ID, request.ID); done <- outcome{result, err} }()
	<-entered
	if err := s.Restore(preview.ID); err != nil {
		t.Fatal(err)
	}
	close(release)
	out := <-done
	if out.err == nil || out.result.Code != "reset" || s.GetState().Accounts[0].PendingReset != nil {
		t.Fatal("消费响应改写恢复数据或丢失服务端结果")
	}
}
