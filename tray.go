package main

import (
	"context"
	"embed"
	"fmt"
	"runtime"
	"time"

	"codex-deck/internal/deck"
	"codex-deck/internal/tray"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
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
	controller, err := tray.Start(icon, tray.Actions{Show: a.showWindow, Quit: a.quit, Refresh: a.refreshFromTray, Switch: a.switchFromTray})
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(a.ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			state := a.service.GetState()
			a.desktopMu.Lock()
			menu := trayMenu(state, time.Now(), a.frontendPaused, a.trayBusy, a.trayMessage)
			a.desktopMu.Unlock()
			controller.Update(menu)
			select {
			case <-ctx.Done():
				return
			case <-a.trayUpdates:
			case <-ticker.C:
			}
		}
	}()
	return func() { cancel(); <-done; controller.Stop() }, nil
}

func (a *App) invalidateTray() {
	select {
	case a.trayUpdates <- struct{}{}:
	default:
	}
}

func (a *App) beginTrayAction(label string) bool {
	a.desktopMu.Lock()
	defer a.desktopMu.Unlock()
	if a.quitting || a.frontendPaused || a.trayBusy != "" {
		return false
	}
	a.trayBusy, a.trayMessage, a.trayFailed = label, "", false
	a.trayWorkers.Add(1)
	return true
}

func (a *App) runTrayAction(label string, action func(*deck.Service) (string, error)) bool {
	if !a.beginTrayAction(label) {
		return false
	}
	defer a.trayWorkers.Done()
	s := a.service
	s.SetAutomaticPaused(true)
	a.emitTrayState()
	message, err := action(s)
	if err != nil {
		message = "托盘操作失败 · " + err.Error()
	}
	a.desktopMu.Lock()
	a.trayBusy, a.trayMessage, a.trayFailed = "", message, err != nil
	s.SetAutomaticPaused(a.frontendPaused)
	a.desktopMu.Unlock()
	a.emitTrayState()
	return true
}

func (a *App) emitTrayState() {
	a.invalidateTray()
	wailsruntime.EventsEmit(a.ctx, "deck:state", a.viewState(a.service.GetState()))
}

func (a *App) refreshFromTray() {
	a.runTrayAction("正在刷新当前账号", func(s *deck.Service) (string, error) {
		state := s.GetState()
		if state.CLIError != "" {
			return "", fmt.Errorf("CLI 同步失败: %s", state.CLIError)
		}
		for _, account := range state.Accounts {
			if account.Current {
				return "", s.RefreshAccount(account.ID)
			}
		}
		return "", fmt.Errorf("未识别当前账号，请在窗口中刷新账号")
	})
}

func (a *App) switchFromTray(id string) {
	confirm := false
	a.runTrayAction("正在切换账号", func(s *deck.Service) (string, error) {
		state := s.GetState()
		found := false
		for _, account := range state.Accounts {
			if account.ID == id {
				found = true
				if account.Current {
					return "", nil
				}
				if account.AuthError != "" {
					return "", fmt.Errorf("账号需要重新登录，请在窗口中处理")
				}
			}
		}
		if !found {
			return "", fmt.Errorf("账号已移除，请重新选择")
		}
		if state.CLIStore != "file" {
			confirm = true
			return "请在窗口中确认文件凭据存储", nil
		}
		if err := s.Switch(id, false); err != nil {
			return "", err
		}
		return "账号已切换 · 请重启 Codex CLI", nil
	})
	if confirm {
		a.showWindow()
		wailsruntime.EventsEmit(a.ctx, "deck:tray-switch", id)
	}
}
