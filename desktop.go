package main

import (
	"context"
	"fmt"
	systemruntime "runtime"

	"codex-deck/internal/autostart"
	"codex-deck/internal/deck"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

func (a *App) activate(data options.SecondInstanceData) {
	if autostart.IsLaunch(data.Args) {
		return
	}
	<-a.ready
	a.showWindow()
}

func (a *App) showWindow() {
	if systemruntime.GOOS == "darwin" {
		runtime.WindowUnminimise(a.ctx)
		runtime.Show(a.ctx)
	} else {
		runtime.WindowShow(a.ctx)
	}
}

func (a *App) quit() {
	a.desktopMu.Lock()
	a.quitting = true
	a.desktopMu.Unlock()
	runtime.Quit(a.ctx)
}

func (a *App) domReady(ctx context.Context) {
	s, err := a.backend()
	if a.startHidden {
		if prepareErr := prepareHiddenWindow(ctx); prepareErr != nil {
			runtime.Show(ctx)
			if s != nil {
				s.SetDesktopError(fmt.Errorf("准备隐藏启动窗口失败: %w", prepareErr))
			}
		}
	}
	if err != nil {
		if a.startHidden {
			a.showWindow()
		}
		return
	}
	stop, trayErr := a.startTray()
	a.desktopMu.Lock()
	a.stopTray, a.trayReady = stop, trayErr == nil
	a.desktopMu.Unlock()
	if trayErr != nil {
		s.SetDesktopError(fmt.Errorf("托盘不可用，关闭窗口将退出应用: %w", trayErr))
		if a.startHidden {
			a.showWindow()
		}
	}
	s.StartAutomatic()
	if s.GetState().Preferences.LowQuotaNotify {
		if err := a.initialiseNotifications(); err != nil {
			s.SetDesktopError(err)
		}
	}
	window := s.WindowState()
	if window == nil {
		return
	}
	screens, err := runtime.ScreenGetAll(ctx)
	if err != nil {
		s.SetDesktopError(fmt.Errorf("恢复窗口位置失败: %w", err))
		return
	}
	width, height, x, y := window.Width, window.Height, window.X, window.Y
	for _, screen := range screens {
		if !screen.IsCurrent {
			continue
		}
		width = min(width, screen.Size.Width)
		height = min(height, screen.Size.Height)
		x = max(0, min(x, screen.Size.Width-width))
		y = max(0, min(y, screen.Size.Height-height))
		break
	}
	runtime.WindowSetSize(ctx, width, height)
	runtime.WindowSetPosition(ctx, x, y)
}

func (a *App) beforeClose(ctx context.Context) bool {
	s, err := a.backend()
	if err != nil {
		return false
	}
	if runtime.WindowIsNormal(ctx) {
		width, height := runtime.WindowGetSize(ctx)
		x, y := runtime.WindowGetPosition(ctx)
		if err := s.SaveWindow(deck.WindowState{Width: width, Height: height, X: x, Y: y}); err != nil {
			s.SetDesktopError(fmt.Errorf("保存窗口状态失败: %w", err))
			a.desktopMu.Lock()
			a.quitting = false
			a.desktopMu.Unlock()
			return true
		}
	}
	a.desktopMu.Lock()
	hide := a.trayReady && !a.quitting
	a.desktopMu.Unlock()
	if hide {
		runtime.Hide(ctx)
	}
	return hide
}

func (a *App) initialiseNotifications() error {
	a.notificationMu.Lock()
	defer a.notificationMu.Unlock()
	if a.notificationsReady {
		return nil
	}
	if !runtime.IsNotificationAvailable(a.ctx) {
		return fmt.Errorf("当前系统无法发送桌面通知")
	}
	if err := runtime.InitializeNotifications(a.ctx); err != nil {
		return fmt.Errorf("初始化通知失败: %w", err)
	}
	a.notificationsReady = true
	return nil
}
func (a *App) notifyLowQuota(body string) error {
	if err := a.initialiseNotifications(); err != nil {
		return err
	}
	return runtime.SendNotification(a.ctx, runtime.NotificationOptions{ID: "codex-deck-low-quota", Title: "Codex Deck", Body: body})
}
