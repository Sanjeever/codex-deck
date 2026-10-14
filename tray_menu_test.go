package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"codex-deck/internal/deck"
)

func TestTrayMenuQuotaPrivacyAndActions(t *testing.T) {
	now := time.Now()
	state := deck.State{CLIStore: "file", Preferences: deck.Preferences{LowQuotaThreshold: 20}, Accounts: []deck.AccountView{
		{ID: "current-id", Email: "current@example.test", AccountID: "workspace-secret", Current: true, Usage: &deck.Usage{QueriedAt: now, FiveHour: &deck.Window{RemainingPercent: 20}}},
		{ID: "next-id", Alias: "另一账号", Usage: &deck.Usage{QueriedAt: now.Add(-5 * time.Minute), FiveHour: &deck.Window{RemainingPercent: 80}}},
		{ID: "expired", AuthError: "需要重新登录"},
	}}
	menu := trayMenu(state, now, false, "", "")
	if menu.Current != "当前 · current@example.test" || menu.Quota != "5H 20.0%（偏低） · Weekly 未知" || !menu.RefreshEnabled {
		t.Fatalf("current: %+v", menu)
	}
	if menu.Accounts[0].Enabled || !menu.Accounts[1].Enabled || menu.Accounts[2].Enabled || !strings.Contains(menu.Accounts[1].Title, "缓存已过期") {
		t.Fatalf("choices: %+v", menu.Accounts)
	}
	state.Preferences.Privacy = true
	state.Accounts[1].Alias = "current@example.test workspace-secret"
	state.CLIError = "current@example.test workspace-secret"
	menu = trayMenu(state, now, false, "", "操作失败 current@example.test workspace-secret")
	data, _ := json.Marshal(menu)
	if strings.Contains(string(data), "current@example.test") || strings.Contains(string(data), "workspace-secret") || menu.Current != "当前 · 账号 current-" {
		t.Fatalf("privacy: %s", data)
	}
	for _, test := range []struct {
		paused bool
		busy   string
	}{{true, ""}, {false, "正在切换账号"}} {
		menu = trayMenu(state, now, test.paused, test.busy, "")
		if menu.RefreshEnabled || menu.Accounts[1].Enabled {
			t.Fatalf("busy actions enabled: %+v", menu)
		}
	}
}

func TestTrayMenuUnknownAndRecoveryExpiresCache(t *testing.T) {
	now := time.Now()
	menu := trayMenu(deck.State{}, now, false, "", "")
	if menu.Current != "未识别当前账号" || menu.RefreshEnabled || !strings.Contains(menu.Quota, "未知") {
		t.Fatalf("empty: %+v", menu)
	}
	reset := now.Add(-time.Second)
	state := deck.State{Accounts: []deck.AccountView{{ID: "current", Current: true, Usage: &deck.Usage{QueriedAt: now.Add(-2 * time.Second), FiveHour: &deck.Window{RemainingPercent: 0, ResetAt: &reset}}}}}
	menu = trayMenu(state, now, false, "", "")
	if !strings.Contains(menu.Quota, "缓存已过期") || !strings.Contains(menu.Quota, "0.0%") {
		t.Fatalf("expired recovery: %+v", menu)
	}
}
