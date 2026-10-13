package main

import (
	"embed"
	"runtime"

	"codex-deck/internal/tray"
)

//go:embed build/appicon.png build/tray.ico
var trayIcons embed.FS

func (a *App) startTray() (func(), error) {
	name := "build/appicon.png"
	if runtime.GOOS == "windows" {
		name = "build/tray.ico"
	}
	icon, err := trayIcons.ReadFile(name)
	if err != nil {
		return nil, err
	}
	return tray.Start(icon, a.showWindow, a.quit)
}
