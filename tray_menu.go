package main

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"codex-deck/internal/deck"
	"codex-deck/internal/tray"
)

var trayEmail = regexp.MustCompile(`[\w.+-]+@[\w.-]+`)

func trayMenu(state deck.State, now time.Time, paused bool, busy, message string) tray.Menu {
	private := func(text string) string {
		if !state.Preferences.Privacy {
			return text
		}
		text = trayEmail.ReplaceAllString(text, "••••@••••")
		for _, account := range state.Accounts {
			if account.Email != "" {
				text = strings.ReplaceAll(text, account.Email, "••••@••••")
			}
			if account.AccountID != "" {
				text = strings.ReplaceAll(text, account.AccountID, "••••••••")
			}
		}
		return text
	}
	threshold := state.Preferences.LowQuotaThreshold
	if threshold == 0 {
		threshold = 10
	}
	quota := func(label string, window *deck.Window) string {
		if window == nil {
			return label + " 未知"
		}
		text := fmt.Sprintf("%s %.1f%%", label, window.RemainingPercent)
		if window.RemainingPercent <= float64(threshold) {
			text += "（偏低）"
		}
		return text
	}
	usage := func(value *deck.Usage) string {
		if value == nil {
			return "5H 未知 · Weekly 未知"
		}
		text := quota("5H", value.FiveHour) + " · " + quota("Weekly", value.Weekly)
		stale := now.Sub(value.QueriedAt) >= 5*time.Minute
		for _, window := range []*deck.Window{value.FiveHour, value.Weekly} {
			if window != nil && window.ResetAt != nil && !window.ResetAt.After(now) && value.QueriedAt.Before(*window.ResetAt) {
				stale = true
			}
		}
		if stale {
			text = "缓存已过期 · " + text
		}
		return text
	}
	menu := tray.Menu{Current: "未识别当前账号", Quota: usage(nil), Updated: "尚未查询", Accounts: []tray.Account{}}
	for _, account := range state.Accounts {
		name := account.Alias
		if name == "" {
			if state.Preferences.Privacy {
				name = "账号 " + account.ID[:min(8, len(account.ID))]
			} else if account.Email != "" {
				name = account.Email
			} else {
				name = account.AccountID
			}
		}
		name = private(name)
		title := name + " · " + usage(account.Usage)
		if account.AuthError != "" {
			title = name + " · 需重新登录"
		}
		menu.Accounts = append(menu.Accounts, tray.Account{ID: account.ID, Title: title, Current: account.Current, Enabled: !paused && busy == "" && !account.Current && account.AuthError == ""})
		if account.Current {
			menu.Current = "当前 · " + name
			menu.Quota = usage(account.Usage)
			menu.RefreshEnabled = !paused && busy == "" && account.AuthError == ""
			if account.Usage != nil {
				menu.Updated = "更新于 " + account.Usage.QueriedAt.Local().Format("01-02 15:04:05")
			}
			if account.AuthError != "" {
				menu.Status = "需重新登录"
			} else if account.UsageError != "" {
				menu.Status = "用量查询失败 · " + private(account.UsageError)
			}
		}
	}
	if state.CLIError != "" {
		menu.Status = "CLI 同步失败 · " + private(state.CLIError)
		menu.RefreshEnabled = false
	}
	if message != "" {
		menu.Status = private(message)
	}
	if busy != "" {
		menu.Status = busy
	} else if paused {
		menu.Status = "请先完成窗口中的操作"
	}
	return menu
}
