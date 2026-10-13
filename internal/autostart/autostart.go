package autostart

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
)

const Flag = "--autostart"
const label = "com.wails.codex-deck.autostart"

type Manager struct {
	executable string
	path       string
}

func IsLaunch(args []string) bool { return slices.Contains(args, Flag) }

func New() (*Manager, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}
	return newManager(executable)
}

func removeRegistration(path string) error {
	err := os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func writeRegistration(path string, data []byte) (err error) {
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".codex-deck-autostart-*")
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, removeRegistration(f.Name())) }()
	_, writeErr := f.Write(data)
	if err = errors.Join(writeErr, f.Close()); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
