package deck

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
)

func existingDirectory(directory string) (string, error) {
	for {
		info, err := os.Stat(directory)
		if err == nil {
			if !info.IsDir() {
				return "", fmt.Errorf("CLI 路径不是目录")
			}
			return directory, nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			return "", err
		}
		directory = parent
	}
}

// 监听目录可捕获 CLI 用临时文件替换 auth.json 的操作。
func (s *Service) StartWatching() error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	directory, err := existingDirectory(s.cli.Home)
	if err == nil {
		err = watcher.Add(directory)
	}
	if err != nil {
		watcher.Close()
		return err
	}
	s.watchWorkers.Add(1)
	go func() {
		defer s.watchWorkers.Done()
		defer watcher.Close()
		timer := time.NewTimer(time.Hour)
		timer.Stop()
		defer timer.Stop()
		for {
			select {
			case <-s.ctx.Done():
				return
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				name := filepath.Clean(event.Name)
				relevant := name == filepath.Join(s.cli.Home, "auth.json") || name == filepath.Join(s.cli.Home, "config.toml") || name == s.cli.Home || strings.HasPrefix(s.cli.Home, name+string(os.PathSeparator))
				if relevant && event.Op&(fsnotify.Create|fsnotify.Write|fsnotify.Remove|fsnotify.Rename) != 0 {
					timer.Reset(150 * time.Millisecond)
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				s.mu.Lock()
				s.watchError = fmt.Sprintf("监听 CLI 认证失败: %v", err)
				s.publish()
				s.mu.Unlock()
			case <-timer.C:
				next, err := existingDirectory(s.cli.Home)
				if err == nil && next != directory {
					err = watcher.Add(next)
					if err == nil {
						watcher.Remove(directory)
						directory = next
					}
				}
				s.CancelAutomatic()
				s.mu.Lock()
				if err != nil {
					s.watchError = fmt.Sprintf("监听 CLI 认证失败: %v", err)
				} else {
					s.watchError = ""
					s.syncCLIAccount(true)
				}
				s.publish()
				s.mu.Unlock()
			}
		}
	}()
	s.mu.Lock()
	s.syncCLIAccount(true)
	s.mu.Unlock()
	return nil
}
