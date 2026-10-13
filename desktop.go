package main

import (
	"context"
	"fmt"

	"codex-deck/internal/deck"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

func (a *App) activate(_ options.SecondInstanceData) {
	<-a.ready
	runtime.WindowUnminimise(a.ctx)
	runtime.WindowShow(a.ctx)
}

func (a *App) domReady(ctx context.Context) {
	s, err := a.backend()
	if err != nil {
		return
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
	if !runtime.WindowIsNormal(ctx) {
		return false
	}
	width, height := runtime.WindowGetSize(ctx)
	x, y := runtime.WindowGetPosition(ctx)
	if err := s.SaveWindow(deck.WindowState{Width: width, Height: height, X: x, Y: y}); err != nil {
		s.SetDesktopError(fmt.Errorf("保存窗口状态失败: %w", err))
		return true
	}
	return false
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
