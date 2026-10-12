package deck

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestLowQuotaNotificationCurrentOnlyAndPersistentDeduplication(t *testing.T) {
	s := testService(t)
	a := testAccount(t)
	addTestAccount(t, s, a)
	if err := s.cli.Write(a.Tokens); err != nil {
		t.Fatal(err)
	}
	var used atomic.Int32
	used.Store(40)
	reset := time.Now().Add(time.Hour)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"plan_type":"plus","rate_limit":{"primary_window":{"limit_window_seconds":18000,"used_percent":%d,"reset_at":%d}}}`, used.Load(), reset.Unix())
	}))
	defer server.Close()
	s.backend = Backend{Client: server.Client(), BaseURL: server.URL}
	notices := make(chan string, 4)
	s.SetNotifier(func(body string) error { notices <- body; return nil })
	// 默认关闭，额度低也不发通知。
	used.Store(95)
	if err := s.RefreshAccount(a.ID); err != nil {
		t.Fatal(err)
	}
	if len(notices) != 0 {
		t.Fatal("默认发送通知")
	}
	if err := s.SavePreferences(Preferences{LowQuotaNotify: true}); err != nil {
		t.Fatal(err)
	}
	used.Store(40)
	s.RefreshAccount(a.ID)
	used.Store(95)
	s.RefreshAccount(a.ID)
	select {
	case body := <-notices:
		if !strings.Contains(body, "5H 剩余 5.0%") || strings.Contains(body, a.Email) || strings.Contains(body, a.AccountID) {
			t.Fatal("通知内容错误或泄露身份")
		}
	case <-time.After(time.Second):
		t.Fatal("跨阈值未通知")
	}
	s.RefreshAccount(a.ID)
	s.notificationWorkers.Wait()
	if len(notices) != 0 {
		t.Fatal("同一窗口重复通知")
	}
	store, err := OpenStore(s.store.path)
	if err != nil {
		t.Fatal(err)
	}
	if !store.Snapshot().Accounts[0].Notifications.FiveHour.Sent {
		t.Fatal("重启去重状态未持久保存")
	}
	// 使用非文件 CLI 存储时，该文件不代表当前登录账号。
	if err := s.cli.EnableFileStore(); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	d := s.store.Snapshot()
	d.Accounts[0].Notifications = QuotaNotifications{}
	s.saveAccount(d.Accounts[0])
	s.mu.Unlock()
	if err := os.WriteFile(filepath.Join(s.cli.Home, "config.toml"), []byte("cli_auth_credentials_store = \"keyring\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	s.RefreshAccount(a.ID)
	s.notificationWorkers.Wait()
	if len(notices) != 0 {
		t.Fatal("非当前账号仍通知")
	}
}

func TestQuotaNoticeResetsPerWindowAndIgnoresUnknownOrExpiredData(t *testing.T) {
	now := time.Now()
	reset := now.Add(time.Hour)
	mark := NotificationMark{}
	if quotaNotice(nil, &mark, now, 10, "5H") != "" {
		t.Fatal("未知额度通知")
	}
	window := &Window{RemainingPercent: 8, ResetAt: &reset}
	if quotaNotice(window, &mark, now, 10, "5H") == "" {
		t.Fatal("低额度未通知")
	}
	// 服务端恢复时间轻微变化不重复发出。
	changed := reset.Add(time.Second)
	window.ResetAt = &changed
	if quotaNotice(window, &mark, now, 10, "5H") != "" {
		t.Fatal("恢复时间轻微变化重复通知")
	}
	next := reset.Add(24 * time.Hour)
	window.ResetAt = &next
	if quotaNotice(window, &mark, reset.Add(time.Minute), 10, "5H") == "" {
		t.Fatal("新窗口未通知")
	}
	if quotaNotice(window, &NotificationMark{}, next.Add(time.Minute), 10, "5H") != "" {
		t.Fatal("已过期窗口通知")
	}
}

func TestNotificationFailureIsVisible(t *testing.T) {
	s := testService(t)
	s.SetNotifier(func(string) error { return errors.New("系统拒绝通知") })
	s.mu.Lock()
	s.sendQuotaNotice("当前账号")
	s.mu.Unlock()
	s.notificationWorkers.Wait()
	if !strings.Contains(s.GetState().DesktopError, "系统拒绝通知") {
		t.Fatal("通知失败被隐藏")
	}
}
