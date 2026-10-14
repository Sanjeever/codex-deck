package main

import (
	"encoding/json"
	"testing"
	"time"

	"codex-deck/internal/deck"
	"codex-deck/internal/network"
	"codex-deck/internal/update"
)

func TestAppStatePreservesJSONContract(t *testing.T) {
	at := time.Date(2026, 10, 14, 8, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name  string
		state deck.State
	}{
		{name: "empty"},
		{
			name: "populated",
			state: deck.State{
				Network:        network.Config{Mode: "custom", URL: "http://localhost:8080"},
				DesktopError:   "托盘不可用",
				Preferences:    deck.Preferences{Privacy: true, Sort: "email", LowQuotaNotify: true, LowQuotaThreshold: 20, DisableUpdateCheck: true},
				Accounts:       []deck.AccountView{{ID: "test-account", Usage: &deck.Usage{QueriedAt: at, FiveHour: &deck.Window{ResetAt: &at}}}},
				Settings:       deck.Settings{WebDAVURL: "http://localhost:8080/backup"},
				CLIStore:       "file",
				CLIError:       "凭据不可用",
				Backup:         deck.BackupStatus{Busy: true, Conflict: true, At: &at, Error: "备份失败"},
				NextRefreshAt:  &at,
				AutoRefreshing: true,
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			app := &App{updateFailure: "更新失败"}
			got, err := json.Marshal(app.viewState(test.state))
			if err != nil {
				t.Fatal(err)
			}
			// 保留原匿名嵌入的 JSON 契约，检查显式映射未遗漏字段或改变空值。
			want, err := json.Marshal(struct {
				deck.State
				TrayBusy  string       `json:"tray_busy"`
				BuildInfo BuildInfo    `json:"build_info"`
				Update    update.State `json:"update"`
			}{test.state, "", currentBuild(), update.State{Error: "更新失败"}})
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != string(want) {
				t.Fatalf("JSON contract changed:\ngot:  %s\nwant: %s", got, want)
			}
		})
	}
}
