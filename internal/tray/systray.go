//go:build windows || linux

package tray

import "github.com/energye/systray"

func configure(icon []byte, actions Actions) func(Menu) {
	systray.SetIcon(icon)
	systray.SetTitle("Codex Deck")
	systray.SetTooltip("Codex Deck")
	systray.SetOnClick(func(systray.IMenu) { go actions.Show() })
	current := systray.AddMenuItem("未识别当前账号", "")
	quota := systray.AddMenuItem("5H 未知 · Weekly 未知", "")
	updated := systray.AddMenuItem("尚未查询", "")
	status := systray.AddMenuItem("", "")
	for _, item := range []*systray.MenuItem{current, quota, updated, status} {
		item.Disable()
	}
	status.Hide()
	refresh := systray.AddMenuItem("刷新当前账号", "")
	refresh.Disable()
	refresh.Click(func() { go actions.Refresh() })
	switchMenu := systray.AddMenuItem("切换账号", "")
	switchMenu.Disable()
	systray.AddMenuItem("显示窗口", "").Click(func() { go actions.Show() })
	systray.AddMenuItem("退出", "").Click(func() { go actions.Quit() })
	accounts := map[string]*systray.MenuItem{}
	return func(menu Menu) {
		current.SetTitle(menu.Current)
		quota.SetTitle(menu.Quota)
		updated.SetTitle(menu.Updated)
		status.SetTitle(menu.Status)
		if menu.Status == "" {
			status.Hide()
		} else {
			status.Show()
		}
		if menu.RefreshEnabled {
			refresh.Enable()
		} else {
			refresh.Disable()
		}
		for _, item := range accounts {
			item.Hide()
		}
		for _, account := range menu.Accounts {
			item := accounts[account.ID]
			if item == nil {
				item = switchMenu.AddSubMenuItemCheckbox(account.Title, "", account.Current)
				id := account.ID
				item.Click(func() { go actions.Switch(id) })
				accounts[id] = item
			}
			item.SetTitle(account.Title)
			if account.Current {
				item.Check()
			} else {
				item.Uncheck()
			}
			if account.Enabled {
				item.Enable()
			} else {
				item.Disable()
			}
			item.Show()
		}
		if len(menu.Accounts) > 0 {
			switchMenu.Enable()
		} else {
			switchMenu.Disable()
		}
	}
}
