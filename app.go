package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"codex-deck/internal/deck"
	"github.com/pkg/browser"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	ctx     context.Context
	ready   chan struct{}
	service *deck.Service
	err     error
	cancel  context.CancelFunc
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
		a.service.SetOnChange(func(state deck.State) { runtime.EventsEmit(ctx, "deck:state", state) })
		a.err = a.service.StartWatching()
	}
}

func (a *App) shutdown(ctx context.Context) {
	<-a.ready
	if a.cancel != nil {
		a.cancel()
	}
	if a.service != nil {
		a.service.Shutdown()
	}
}

func (a *App) backend() (*deck.Service, error) { <-a.ready; return a.service, a.err }

func (a *App) GetState() (deck.State, error) {
	s, err := a.backend()
	if err != nil {
		return deck.State{}, err
	}
	return s.GetState(), nil
}
func (a *App) RefreshAll() (deck.State, error) {
	s, err := a.backend()
	if err != nil {
		return deck.State{}, err
	}
	return s.RefreshAll(), nil
}
func (a *App) RefreshAutomatic() (deck.State, error) {
	s, err := a.backend()
	if err != nil {
		return deck.State{}, err
	}
	return s.RefreshAutomatic(), nil
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

func (a *App) CancelAutomatic() error {
	s, err := a.backend()
	if err != nil {
		return err
	}
	s.CancelAutomatic()
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
	return s.SavePreferences(p)
}
