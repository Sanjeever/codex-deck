package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"codex-deck/internal/deck"
	"codex-deck/internal/update"
	"github.com/pkg/browser"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	updater            *update.Manager
	updateFailure      string
	updateInstalling   bool
	autoStartMu        sync.Mutex
	startHidden        bool
	desktopMu          sync.Mutex
	stopTray           func()
	trayReady          bool
	quitting           bool
	notificationMu     sync.Mutex
	notificationsReady bool
	ctx                context.Context
	ready              chan struct{}
	service            *deck.Service
	err                error
	cancel             context.CancelFunc
}

func NewApp() *App { return &App{ready: make(chan struct{})} }
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	defer close(a.ready)
	ctx, a.cancel = context.WithCancel(ctx)
	home, err := os.UserHomeDir()
	if err != nil {
		a.err = err
		return
	}
	cliHome := os.Getenv("CODEX_HOME")
	if cliHome == "" {
		cliHome = filepath.Join(home, ".codex")
	}
	a.service, a.err = deck.NewService(ctx, filepath.Join(home, ".codex-deck.json"), cliHome)
	if a.err == nil {
		a.service.SetNotifier(a.notifyLowQuota)
		a.updater = update.New(currentBuild().Version, func() { runtime.EventsEmit(ctx, "deck:state", a.viewState(a.service.GetState())) })
		a.updater.Client.Transport = a.service.HTTP
		a.service.SetOnChange(func(state deck.State) { runtime.EventsEmit(ctx, "deck:state", a.viewState(state)) })
		a.err = a.service.StartWatching()
	}
}

func (a *App) shutdown(ctx context.Context) {
	defer func() {
		if a.updater != nil {
			if err := a.updater.ReleaseParent(); err != nil {
				println(err.Error())
			}
		}
	}()
	<-a.ready
	if a.updater != nil {
		a.updater.Stop()
	}
	if a.cancel != nil {
		a.cancel()
	}
	if a.service != nil {
		a.service.Shutdown()
		if a.notificationsReady {
			runtime.CleanupNotifications(a.ctx)
		}
	}
	a.desktopMu.Lock()
	stopTray := a.stopTray
	a.desktopMu.Unlock()
	if stopTray != nil {
		stopTray()
	}
}

func (a *App) backend() (*deck.Service, error) { <-a.ready; return a.service, a.err }

func (a *App) GetState() (AppState, error) {
	s, err := a.backend()
	if err != nil {
		return AppState{}, err
	}
	return a.viewState(s.GetState()), nil
}
func (a *App) RefreshAll() (AppState, error) {
	s, err := a.backend()
	if err != nil {
		return AppState{}, err
	}
	return a.viewState(s.RefreshAll()), nil
}

type AppState struct {
	deck.State
	BuildInfo BuildInfo    `json:"build_info"`
	Update    update.State `json:"update"`
}

func (a *App) viewState(state deck.State) AppState {
	result := AppState{State: state, BuildInfo: currentBuild()}
	if a.updater != nil {
		result.Update = a.updater.State()
	}
	a.desktopMu.Lock()
	result.Update.Error = firstUpdateError(result.Update.Error, a.updateFailure)
	a.desktopMu.Unlock()
	return result
}
func firstUpdateError(current, startup string) string {
	if startup != "" {
		return startup
	}
	return current
}
func (a *App) CheckUpdate() error {
	if _, err := a.backend(); err != nil {
		return err
	}
	return a.updater.Check(a.ctx)
}
func (a *App) InstallUpdate() error {
	if _, err := a.backend(); err != nil {
		return err
	}
	if currentBuild().Development {
		return fmt.Errorf("开发版不能自动安装更新")
	}
	go func() {
		if err := a.updater.Start(a.ctx, func() error {
			if runtime.WindowIsNormal(a.ctx) {
				width, height := runtime.WindowGetSize(a.ctx)
				x, y := runtime.WindowGetPosition(a.ctx)
				if err := a.service.SaveWindow(deck.WindowState{Width: width, Height: height, X: x, Y: y}); err != nil {
					return err
				}
			}
			return nil
		}, func() { a.desktopMu.Lock(); a.updateInstalling = true; a.desktopMu.Unlock(); a.quit() }); err != nil {
			println(err.Error())
		}
	}()
	return nil
}
func (a *App) CancelUpdate() {
	if a.updater != nil {
		a.updater.Cancel()
	}
}
func (a *App) startUpdateChecks() {
	if currentBuild().Development {
		return
	}
	go func() {
		timer := time.NewTimer(30 * time.Second)
		defer timer.Stop()
		for {
			select {
			case <-a.ctx.Done():
				return
			case <-timer.C:
				if !a.service.GetState().Preferences.DisableUpdateCheck {
					if err := a.updater.Check(a.ctx); err != nil {
						println(err.Error())
					}
				}
				timer.Reset(6 * time.Hour)
			}
		}
	}()
}
func (a *App) SetAutomaticPaused(paused bool) error {
	s, err := a.backend()
	if err != nil {
		return err
	}
	s.SetAutomaticPaused(paused)
	return nil
}
func (a *App) WakeAutomatic() error {
	s, err := a.backend()
	if err != nil {
		return err
	}
	s.WakeAutomatic()
	return nil
}
func (a *App) RefreshAccount(id string) error {
	s, err := a.backend()
	if err != nil {
		return err
	}
	return s.RefreshAccount(id)
}
func (a *App) RefreshCredits(id string) error {
	s, err := a.backend()
	if err != nil {
		return err
	}
	return s.RefreshCredits(id)
}
func (a *App) Rename(id, alias string) error {
	s, err := a.backend()
	if err != nil {
		return err
	}
	return s.Rename(id, alias)
}
func (a *App) Delete(id string) error {
	s, err := a.backend()
	if err != nil {
		return err
	}
	return s.Delete(id)
}
func (a *App) Switch(id string, enableFile bool) error {
	s, err := a.backend()
	if err != nil {
		return err
	}
	return s.Switch(id, enableFile)
}
func (a *App) SaveSettings(settings deck.Settings) error {
	s, err := a.backend()
	if err != nil {
		return err
	}
	return s.SaveSettings(settings)
}
func (a *App) Backup() error {
	s, err := a.backend()
	if err != nil {
		return err
	}
	return s.Backup()
}
func (a *App) PrepareRestore() (deck.RestorePreview, error) {
	s, err := a.backend()
	if err != nil {
		return deck.RestorePreview{}, err
	}
	return s.PrepareRestore()
}
func (a *App) CancelRestore() error {
	s, err := a.backend()
	if err != nil {
		return err
	}
	s.CancelRestore()
	return nil
}
func (a *App) Restore(id string) error {
	s, err := a.backend()
	if err != nil {
		return err
	}
	return s.Restore(id)
}
func (a *App) PrepareConsume(id, creditID string) (deck.ResetRequest, error) {
	s, err := a.backend()
	if err != nil {
		return deck.ResetRequest{}, err
	}
	return s.PrepareConsume(id, creditID)
}
func (a *App) Consume(id, requestID string) (deck.ConsumeResult, error) {
	s, err := a.backend()
	if err != nil {
		return deck.ConsumeResult{}, err
	}
	return s.Consume(id, requestID)
}

type LoginInfo struct {
	ID string `json:"id"`
}

func (a *App) BeginLogin(target string) (LoginInfo, error) {
	s, err := a.backend()
	if err != nil {
		return LoginInfo{}, err
	}
	l, err := s.BeginLogin(target)
	if err != nil {
		return LoginInfo{}, err
	}
	if err = browser.OpenURL(l.URL); err != nil {
		s.CancelLogin(l.ID)
		finishErr := s.WaitLogin(l.ID)
		return LoginInfo{}, fmt.Errorf("打开浏览器失败: %v；%v", err, finishErr)
	}
	return LoginInfo{ID: l.ID}, nil
}
func (a *App) WaitLogin(id string) error {
	s, err := a.backend()
	if err != nil {
		return err
	}
	return s.WaitLogin(id)
}
func (a *App) CancelLogin(id string) error {
	s, err := a.backend()
	if err != nil {
		return err
	}
	s.CancelLogin(id)
	return nil
}

func (a *App) OverwriteBackup() error {
	s, err := a.backend()
	if err != nil {
		return err
	}
	return s.OverwriteBackup()
}

func (a *App) SavePreferences(p deck.Preferences) error {
	s, err := a.backend()
	if err != nil {
		return err
	}
	if err := a.authorizeNotifications(p.LowQuotaNotify, s.GetState().Preferences.LowQuotaNotify); err != nil {
		return err
	}
	return s.SavePreferences(p)
}

func (a *App) authorizeNotifications(enabled, previouslyEnabled bool) error {
	if enabled && !previouslyEnabled {
		if err := a.initialiseNotifications(); err != nil {
			return err
		}
		granted, err := runtime.RequestNotificationAuthorization(a.ctx)
		if err != nil {
			return fmt.Errorf("请求通知权限失败: %w", err)
		}
		if !granted {
			return fmt.Errorf("通知权限未开启，请在系统设置中允许 Codex Deck 发送通知")
		}
	}
	return nil
}
