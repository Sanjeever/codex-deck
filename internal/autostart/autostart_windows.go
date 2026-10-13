package autostart

import (
	"errors"
	"fmt"
	"unicode/utf16"

	"golang.org/x/sys/windows/registry"
)

const valueName = "Codex Deck"

func newManager(executable string) (*Manager, error) {
	return &Manager{executable: executable, path: `Software\Microsoft\Windows\CurrentVersion\Run`}, nil
}

func (m *Manager) Enabled() (bool, error) {
	key, err := registry.OpenKey(registry.CURRENT_USER, m.path, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer key.Close()
	command, _, err := key.GetStringValue(valueName)
	if errors.Is(err, registry.ErrNotExist) {
		return false, nil
	}
	return command != "", err
}

func (m *Manager) Set(enabled bool) error {
	if !enabled {
		key, err := registry.OpenKey(registry.CURRENT_USER, m.path, registry.SET_VALUE)
		if errors.Is(err, registry.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		defer key.Close()
		err = key.DeleteValue(valueName)
		if errors.Is(err, registry.ErrNotExist) {
			return nil
		}
		return err
	}
	command := `"` + m.executable + `" ` + Flag
	if len(utf16.Encode([]rune(command))) > 260 {
		return fmt.Errorf("自启动命令超过 Windows Run 项的 260 字符限制")
	}
	key, _, err := registry.CreateKey(registry.CURRENT_USER, m.path, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
	return key.SetStringValue(valueName, command)
}
