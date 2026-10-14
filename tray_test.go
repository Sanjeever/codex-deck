package main

import (
	"context"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"codex-deck/internal/deck"
)

func TestTrayActionsExcludeWindowWorkAndConcurrentClicks(t *testing.T) {
	app := NewApp()
	service, err := deck.NewService(context.Background(), filepath.Join(t.TempDir(), "deck.json"), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(service.Shutdown)
	app.service = service
	close(app.ready)
	if err := app.SetAutomaticPaused(true); err != nil {
		t.Fatal(err)
	}
	if app.beginTrayAction("刷新") {
		t.Fatal("window operation did not exclude tray action")
	}
	if err := app.SetAutomaticPaused(false); err != nil {
		t.Fatal(err)
	}
	var accepted atomic.Int32
	var clicks sync.WaitGroup
	for range 20 {
		clicks.Add(1)
		go func() {
			defer clicks.Done()
			if app.beginTrayAction("正在切换账号") {
				accepted.Add(1)
			}
		}()
	}
	clicks.Wait()
	defer app.trayWorkers.Done()
	if accepted.Load() != 1 || app.viewState(service.GetState()).TrayBusy != "正在切换账号" {
		t.Fatal("concurrent clicks accepted more than one action or lost busy state")
	}
	if err := app.SetAutomaticPaused(true); err == nil {
		t.Fatal("window operation entered during tray action")
	}
	if err := app.SetAutomaticPaused(false); err != nil {
		t.Fatal(err)
	}
	var broadcasts atomic.Int32
	service.SetOnChange(func(deck.State) { broadcasts.Add(1) })
	service.RefreshAutomatic()
	if broadcasts.Load() != 0 {
		t.Fatal("window unpause resumed automatic work during tray action")
	}
}

func TestTrayShutdownRejectsNewActionsAndPreservesDesktopErrors(t *testing.T) {
	app := NewApp()
	app.quitting = true
	if app.beginTrayAction("刷新") {
		t.Fatal("tray action accepted during shutdown")
	}
	app.trayFailed, app.trayMessage = true, "托盘操作失败 · 写入失败"
	state := app.viewState(deck.State{DesktopError: "通知不可用"})
	if state.DesktopError != "通知不可用；托盘操作失败 · 写入失败" {
		t.Fatalf("lost errors: %s", state.DesktopError)
	}
}
