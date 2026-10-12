package main

import (
	"embed"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := NewApp()
	if err := wails.Run(&options.App{
		Title: "Codex Deck", Width: 1120, Height: 720, MinWidth: 860, MinHeight: 500,
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
