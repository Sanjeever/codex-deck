package main

import (
	"fmt"

	"codex-deck/internal/autostart"
)

func (a *App) GetAutoStart() (bool, error) {
	a.autoStartMu.Lock()
	defer a.autoStartMu.Unlock()
	m, err := autostart.New()
	if err != nil {
		return false, err
	}
	enabled, err := m.Enabled()
	if err != nil {
		return false, fmt.Errorf("读取开机自启动设置失败: %w", err)
	}
	return enabled, nil
}

func (a *App) SetAutoStart(enabled bool) (bool, error) {
	a.autoStartMu.Lock()
	defer a.autoStartMu.Unlock()
	m, err := autostart.New()
	if err != nil {
		return false, err
	}
	if err := m.Set(enabled); err != nil {
		return false, fmt.Errorf("修改开机自启动设置失败: %w", err)
	}
	return m.Enabled()
}
