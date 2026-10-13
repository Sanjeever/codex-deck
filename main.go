package main

import (
	"embed"
	"os"
	"strconv"

	"codex-deck/internal/autostart"
	"codex-deck/internal/update"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--apply-update" {
		if err := update.Apply(os.Args[2:]); err != nil {
			println(err.Error())
			os.Exit(1)
		}
		return
	}
	app := NewApp()
	for i := 1; i < len(os.Args); i++ {
		switch os.Args[i] {
		case "--update-cleanup":
			if i+2 >= len(os.Args) {
				println("更新清理参数无效")
				return
			}
			stage := os.Args[i+1]
			pid, err := strconv.Atoi(os.Args[i+2])
			if err != nil {
				println(err.Error())
				return
			}
			i += 2
			go func() {
				if err := update.Cleanup(stage, pid); err != nil {
					app.desktopMu.Lock()
					app.updateFailure = err.Error()
					app.desktopMu.Unlock()
				}
			}()
		case "--update-error":
			if i+1 >= len(os.Args) {
				println("更新错误参数无效")
				return
			}
			app.desktopMu.Lock()
			app.updateFailure = os.Args[i+1]
			app.desktopMu.Unlock()
			i++
		}
	}
	app.startHidden = autostart.IsLaunch(os.Args[1:])
	if err := wails.Run(&options.App{
		Title: "Codex Deck", Width: 1120, Height: 720, MinWidth: 860, MinHeight: 500,
		StartHidden:        app.startHidden,
		AssetServer:        &assetserver.Options{Assets: assets},
		SingleInstanceLock: &options.SingleInstanceLock{UniqueId: "codex-deck", OnSecondInstanceLaunch: app.activate},
		OnStartup:          app.startup,
		OnDomReady:         app.domReady,
		OnBeforeClose:      app.beforeClose,
		OnShutdown:         app.shutdown,
		Bind:               []interface{}{app},
	}); err != nil {
		println(err.Error())
	}
}
