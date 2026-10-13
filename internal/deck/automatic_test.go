package deck

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestAutomaticRefreshCacheFailuresAndManualOverride(t *testing.T) {
	s := testService(t)
	a := testAccount(t)
	a.Usage = &Usage{Plan: "plus", FiveHour: &Window{RemainingPercent: 42}, QueriedAt: time.Now()}
	addTestAccount(t, s, a)
	var requests atomic.Int32
	var fail atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if fail.Load() {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.Write([]byte(`{"plan_type":"plus"}`))
	}))
	defer server.Close()
	s.backend = Backend{Client: server.Client(), BaseURL: server.URL}
	if state := s.RefreshAutomatic(); requests.Load() != 0 || len(state.Accounts) != 1 || state.Accounts[0].Usage.FiveHour.RemainingPercent != 42 {
		t.Fatal("未到期缓存仍发请求或清空数据")
	}
	if err := s.store.Update(func(d *Data) error { d.Accounts[0].Usage.QueriedAt = time.Now().Add(-autoRefreshInterval); return nil }); err != nil {
		t.Fatal(err)
	}
	fail.Store(true)
	state := s.RefreshAutomatic()
	if requests.Load() != 1 || state.Accounts[0].UsageError == "" || state.Accounts[0].Usage.FiveHour.RemainingPercent != 42 || state.NextRefreshAt.Before(time.Now().Add(4*time.Minute)) {
		t.Fatal("失败未保留缓存或设置下次查询时间")
	}
	s.RefreshAutomatic()
	if requests.Load() != 1 {
		t.Fatal("失败后立即重复请求")
	}
	fail.Store(false)
	state = s.RefreshAll()
	if requests.Load() != 2 || state.Accounts[0].UsageError != "" {
		t.Fatal("手动刷新未绕过间隔或清除失败状态")
	}
	s.RefreshAutomatic()
	if requests.Load() != 2 {
		t.Fatal("手动刷新后自动重复查询")
	}
	s.usageTries[a.ID] = time.Now().Add(-autoRefreshInterval)
	if err := s.store.Update(func(d *Data) error { d.Accounts[0].Usage.QueriedAt = time.Now().Add(-autoRefreshInterval); return nil }); err != nil {
		t.Fatal(err)
	}
	s.RefreshAutomatic()
	if requests.Load() != 3 {
		t.Fatal("下一周期未查询")
	}
}

func TestAutomaticRefreshAtQuotaReset(t *testing.T) {
	s := testService(t)
	a := testAccount(t)
	reset := time.Now().Add(-time.Second)
	a.Usage = &Usage{Plan: "plus", FiveHour: &Window{RemainingPercent: 0, ResetAt: &reset}, QueriedAt: reset.Add(-time.Minute)}
	addTestAccount(t, s, a)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		json.NewEncoder(w).Encode(map[string]any{"plan_type": "plus", "rate_limit": map[string]any{"primary_window": map[string]any{"limit_window_seconds": 18000, "used_percent": 15, "reset_at": reset.Unix()}}})
	}))
	defer server.Close()
	s.backend = Backend{Client: server.Client(), BaseURL: server.URL}
	state := s.RefreshAutomatic()
	if requests.Load() != 1 || state.Accounts[0].Usage.FiveHour.RemainingPercent != 85 {
		t.Fatal("恢复时间后未查询真实剩余额度")
	}
	s.RefreshAutomatic()
	if requests.Load() != 1 {
		t.Fatal("服务端旧恢复时间造成查询循环")
	}
}

func TestAutomaticRefreshPreservesDeletionAndRestore(t *testing.T) {
	s := testService(t)
	a := testAccount(t)
	if err := s.cli.Write(a.Tokens); err != nil {
		t.Fatal(err)
	}
	restarted, err := NewService(context.Background(), s.store.path, s.cli.Home)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(restarted.Shutdown)
	id := restarted.GetState().Accounts[0].ID
	if err := os.Remove(filepath.Join(s.cli.Home, "auth.json")); err != nil {
		t.Fatal(err)
	}
	if err := restarted.Delete(id); err != nil {
		t.Fatal(err)
	}
	if err := s.cli.Write(a.Tokens); err != nil {
		t.Fatal(err)
	}
	if state := restarted.RefreshAutomatic(); len(state.Accounts) != 0 {
		t.Fatal("自动刷新重新加入已删除账号")
	}
	settings, _, closeServer := testWebDAV(t)
	defer closeServer()
	if err := restarted.SaveSettings(settings); err != nil {
		t.Fatal(err)
	}
	waitBackup(t, restarted)
	if err := uploadBackup(Data{Version: Version, Settings: settings, Accounts: []Account{}}); err != nil {
		t.Fatal(err)
	}
	preview, err := restarted.PrepareRestore()
	if err != nil {
		t.Fatal(err)
	}
	if err := restarted.Restore(preview.ID); err != nil {
		t.Fatal(err)
	}
	if state := restarted.RefreshAutomatic(); len(state.Accounts) != 0 {
		t.Fatal("恢复后自动合并原 CLI 账号")
	}
	other := testAccount(t)
	other.UserID = "other-user"
	other.AccountID = "other-workspace"
	other.Tokens.AccountID = other.AccountID
	other.Tokens.IDToken = testToken(t, other.UserID, other.AccountID, time.Now().Add(time.Hour))
	other.Tokens.AccessToken = other.Tokens.IDToken
	if err := restarted.cli.Write(other.Tokens); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"plan_type":"plus"}`)) }))
	defer server.Close()
	restarted.backend = Backend{Client: server.Client(), BaseURL: server.URL}
	state := restarted.RefreshAutomatic()
	if len(state.Accounts) != 1 || state.Accounts[0].AccountID != other.AccountID || !state.Accounts[0].Current || state.Accounts[0].Usage == nil {
		t.Fatal("未自动发现和查询新 CLI 账号")
	}
}

func TestNextUsageRefreshDeadline(t *testing.T) {
	now := time.Now()
	reset := now.Add(time.Minute)
	a := Account{Usage: &Usage{QueriedAt: now, Weekly: &Window{ResetAt: &reset}}}
	if next := nextUsageRefresh(a, time.Time{}); !next.Equal(reset) {
		t.Fatal("未采用更近的额度恢复时间")
	}
	attempted := reset.Add(time.Second)
	if next := nextUsageRefresh(a, attempted); !next.Equal(attempted.Add(autoRefreshInterval)) {
		t.Fatal("已尝试的恢复窗口被重复查询")
	}
}

func TestDeleteCurrentAccountAfterDeckSwitch(t *testing.T) {
	s := testService(t)
	a := testAccount(t)
	addTestAccount(t, s, a)
	if err := s.Switch(a.ID, false); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(a.ID); err == nil {
		t.Fatal("切换后的当前账号仍可删除")
	}
	if state := s.GetState(); len(state.Accounts) != 1 || !state.Accounts[0].Current {
		t.Fatal("拒绝删除未保留当前账号")
	}
}

func TestAutomaticSchedulerRefreshesAtResetAndNotifiesWithoutFrontend(t *testing.T) {
	s := testService(t)
	a := testAccount(t)
	reset := time.Now().Add(150 * time.Millisecond)
	a.Usage = &Usage{FiveHour: &Window{RemainingPercent: 20, ResetAt: &reset}, QueriedAt: time.Now()}
	addTestAccount(t, s, a)
	if err := s.cli.Write(a.Tokens); err != nil {
		t.Fatal(err)
	}
	if err := s.SavePreferences(Preferences{LowQuotaNotify: true}); err != nil {
		t.Fatal(err)
	}
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Write([]byte(`{"plan_type":"plus","rate_limit":{"primary_window":{"limit_window_seconds":18000,"used_percent":95}}}`))
	}))
	defer server.Close()
	s.backend = Backend{Client: server.Client(), BaseURL: server.URL}
	notices := make(chan string, 2)
	s.SetNotifier(func(body string) error { notices <- body; return nil })
	completed := make(chan State, 8)
	s.SetOnChange(func(state State) {
		if !state.AutoRefreshing && state.Accounts[0].Usage.FiveHour.RemainingPercent == 5 {
			completed <- state
		}
	})
	s.StartAutomatic()
	select {
	case body := <-notices:
		if !strings.Contains(body, "5H 剩余 5.0%") {
			t.Fatal(body)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("无前端调用时未按恢复时间查询并通知")
	}
	select {
	case state := <-completed:
		if state.NextRefreshAt.Before(time.Now().Add(4 * time.Minute)) {
			t.Fatal("刷新后未沿用五分钟间隔")
		}
	case <-time.After(time.Second):
		t.Fatal("未发布后台查询结果")
	}
	s.WakeAutomatic()
	select {
	case <-completed:
	case <-time.After(time.Second):
		t.Fatal("调度未响应唤醒")
	}
	if requests.Load() != 1 || len(notices) != 0 {
		t.Fatal("唤醒重复查询未到期缓存或重复通知")
	}
}

func TestAutomaticSchedulerPauseCancelsRequestAndResumes(t *testing.T) {
	s := testService(t)
	a := testAccount(t)
	addTestAccount(t, s, a)
	entered := make(chan struct{}, 4)
	stopped := make(chan struct{}, 1)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := requests.Add(1)
		entered <- struct{}{}
		if n == 1 {
			<-r.Context().Done()
			stopped <- struct{}{}
			return
		}
		w.Write([]byte(`{"plan_type":"plus"}`))
	}))
	defer server.Close()
	s.backend = Backend{Client: server.Client(), BaseURL: server.URL}
	completed := make(chan State, 4)
	s.SetOnChange(func(state State) {
		if !state.AutoRefreshing {
			completed <- state
		}
	})
	s.StartAutomatic()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("未启动自动查询")
	}
	s.SetAutomaticPaused(true)
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("暂停未取消用量请求")
	}
	<-completed
	if err := s.Rename(a.ID, "操作期间修改"); err != nil {
		t.Fatal(err)
	}
	s.WakeAutomatic()
	select {
	case <-entered:
		t.Fatal("暂停期间仍发出请求")
	case <-time.After(50 * time.Millisecond):
	}
	s.SetAutomaticPaused(false)
	select {
	case state := <-completed:
		if state.Accounts[0].Usage == nil || state.Accounts[0].Alias != "操作期间修改" || state.Accounts[0].UsageError != "" {
			t.Fatal("恢复查询未保留用户修改或取消污染状态")
		}
	case <-time.After(time.Second):
		t.Fatal("操作结束后未恢复查询")
	}
}

func TestAutomaticSchedulerShutdownCancelsInFlightRequest(t *testing.T) {
	s := testService(t)
	addTestAccount(t, s, testAccount(t))
	entered := make(chan struct{})
	stopped := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-r.Context().Done()
		close(stopped)
	}))
	defer server.Close()
	s.backend = Backend{Client: server.Client(), BaseURL: server.URL}
	s.StartAutomatic()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("未启动自动查询")
	}
	done := make(chan struct{})
	go func() { s.Shutdown(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("退出仍在等待用量请求")
	}
	<-stopped
}
