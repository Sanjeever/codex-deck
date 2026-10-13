package autostart

import (
	"os"
	"path/filepath"
)

func newManager(executable string) (*Manager, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	return &Manager{executable: executable, path: filepath.Join(home, "Library", "LaunchAgents", label+".plist")}, nil
}

func (m *Manager) Enabled() (bool, error) { return m.plistEnabled() }
func (m *Manager) Set(enabled bool) error { return m.setPlist(enabled) }
