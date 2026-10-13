//go:build windows || linux

package tray

import "github.com/energye/systray"

func configure(icon []byte, show, quit func()) {
	systray.SetIcon(icon)
	systray.SetTitle("Codex Deck")
	systray.SetTooltip("Codex Deck")
	systray.SetOnClick(func(systray.IMenu) { go show() })
	systray.AddMenuItem("显示窗口", "").Click(func() { go show() })
	systray.AddMenuItem("退出", "").Click(func() { go quit() })
}
