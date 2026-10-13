package autostart

import (
	"os"
	"path/filepath"
)

func newManager(executable string) (*Manager, error) {
	config, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	return &Manager{executable: executable, path: filepath.Join(config, "autostart", "codex-deck.desktop")}, nil
}

func (m *Manager) Enabled() (bool, error) { return m.desktopEnabled() }
func (m *Manager) Set(enabled bool) error { return m.setDesktop(enabled) }
