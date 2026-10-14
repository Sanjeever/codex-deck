package deck

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func sameAuthPath(left, right string) (bool, error) {
	paths := []string{left, right}
	for index, path := range paths {
		absolute, err := filepath.Abs(path)
		if err != nil {
			return false, err
		}
		parent, err := filepath.EvalSymlinks(filepath.Dir(absolute))
		if err != nil && !os.IsNotExist(err) {
			return false, err
		}
		if err == nil {
			absolute = filepath.Join(parent, filepath.Base(absolute))
		}
		paths[index] = absolute
	}
	if paths[0] == paths[1] || (runtime.GOOS == "windows" && strings.EqualFold(paths[0], paths[1])) {
		return true, nil
	}
	leftInfo, leftErr := os.Stat(left)
	rightInfo, rightErr := os.Stat(right)
	if leftErr != nil && !os.IsNotExist(leftErr) {
		return false, leftErr
	}
	if rightErr != nil && !os.IsNotExist(rightErr) {
		return false, rightErr
	}
	return leftErr == nil && rightErr == nil && os.SameFile(leftInfo, rightInfo), nil
}

func (s *Service) IsCLIAuthPath(path string) (bool, error) {
	return sameAuthPath(path, filepath.Join(s.cli.Home, "auth.json"))
}

// 导出只读取已有凭据，不发起令牌刷新。当前账号先同步 CLI 最新凭据。
func (s *Service) ExportAuth(id, path string, overwriteCLI bool) error {
	s.CancelAutomatic()
	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	configTarget, err := sameAuthPath(path, s.store.path)
	if err != nil {
		return err
	}
	if configTarget {
		return fmt.Errorf("不能覆盖 Codex Deck 配置文件")
	}
	cliTarget, err := s.IsCLIAuthPath(path)
	if err != nil {
		return err
	}
	if cliTarget {
		if !overwriteCLI {
			return fmt.Errorf("请确认覆盖 CLI 当前凭据")
		}
		mode, err := s.cli.StoreMode()
		if err != nil {
			return err
		}
		if mode != "file" {
			return fmt.Errorf("CLI 当前使用 %s 存储，请先通过切换账号改为文件存储", mode)
		}
	}
	account, err := findAccount(s.store.Snapshot(), id)
	if err != nil {
		return err
	}
	local, current, err := s.cli.ImportCurrent(account)
	if err != nil {
		return err
	}
	if current {
		account, err = s.syncAccountCredentials(account, local)
		if err != nil {
			return err
		}
	}
	if account.AuthError != "" {
		return fmt.Errorf("账号认证已失效，请重新登录后导出")
	}
	data, err := encodeCLIAuth(account.Tokens)
	if err != nil {
		return err
	}
	if cliTarget {
		if err := s.cli.Write(account.Tokens); err != nil {
			return err
		}
		if err := s.store.Update(func(d *Data) error { d.LastSwitchedID = id; return nil }); err != nil {
			return fmt.Errorf("CLI 凭据已导出，本地状态保存失败: %w", err)
		}
		s.cliIdentity = Identity{UserID: account.UserID, AccountID: account.AccountID, Email: account.Email, Plan: account.Plan}
		s.importError = ""
		s.autoBackup()
		return nil
	}
	if err := atomicWrite(path, data); err != nil {
		return fmt.Errorf("导出认证文件失败: %w", err)
	}
	return nil
}
