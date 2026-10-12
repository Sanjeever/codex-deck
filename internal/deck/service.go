package deck

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

type Service struct {
	mu           sync.Mutex
	store        *Store
	cli          CLI
	oauth        OAuth
	backend      Backend
	ctx          context.Context
	usageErrors  map[string]string
	creditErrors map[string]string
	importError  string
	backup       BackupStatus
	restore      *Data
	restoreID    string
	loginMu      sync.Mutex
	login        *Login
	loginTarget  string
}

func NewService(ctx context.Context, path, cliHome string) (*Service, error) {
	store, err := OpenStore(path)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 30 * time.Second}
	s := &Service{store: store, cli: CLI{Home: cliHome}, oauth: OAuth{Client: client, Issuer: "https://auth.openai.com"}, backend: Backend{Client: client, BaseURL: "https://chatgpt.com/backend-api"}, ctx: ctx, usageErrors: map[string]string{}, creditErrors: map[string]string{}}
	if len(store.Snapshot().Accounts) == 0 {
		a, err := s.cli.ReadAccount()
		if err == nil && a != nil {
			err = s.acceptLogin("", a.Tokens, Identity{UserID: a.UserID, AccountID: a.AccountID, Email: a.Email, Plan: a.Plan})
			if err == nil {
				s.autoBackup()
			}
		}
		if err != nil {
			s.importError = fmt.Sprintf("自动导入 CLI 账号失败: %v", err)
		}
	}
	return s, nil
}

func findAccount(d Data, id string) (Account, error) {
	for _, a := range d.Accounts {
		if a.ID == id {
			return a, nil
		}
	}
	return Account{}, fmt.Errorf("账号不存在")
}

func (s *Service) saveAccount(a Account) error {
	return s.store.Update(func(d *Data) error {
		for n := range d.Accounts {
			if d.Accounts[n].ID == a.ID {
				d.Accounts[n] = a
				return nil
			}
		}
		return fmt.Errorf("账号不存在")
	})
}

func (s *Service) state() State {
	d := s.store.Snapshot()
	state := State{Accounts: []AccountView{}, Settings: d.Settings, Backup: s.backup}
	if len(d.Accounts) == 0 {
		state.CLIError = s.importError
	}
	mode, err := s.cli.StoreMode()
	state.CLIStore = mode
	var current Identity
	if err != nil {
		state.CLIError = err.Error()
	} else if mode == "file" {
		_, i, err := s.cli.Read()
		if err != nil {
			state.CLIError = err.Error()
		} else {
			current = i
		}
	}
	for _, a := range d.Accounts {
		plan := a.Plan
		if a.Usage != nil && a.Usage.Plan != "" {
			plan = a.Usage.Plan
		}
		state.Accounts = append(state.Accounts, AccountView{ID: a.ID, AccountID: a.AccountID, Email: a.Email, Alias: a.Alias, Plan: plan, Current: mode == "file" && identityMatches(a, current), Usage: a.Usage, Credits: a.Credits, UsageError: s.usageErrors[a.ID], CreditsError: s.creditErrors[a.ID], PendingReset: a.PendingReset})
	}
	return state
}

func (s *Service) GetState() State { s.mu.Lock(); defer s.mu.Unlock(); return s.state() }

// 所有变更和上传持有同一把锁，旧备份无法覆盖后续变更。
func (s *Service) autoBackup() {
	if s.store.Snapshot().Settings.WebDAVURL != "" {
		s.backupNow()
	}
}
func (s *Service) backupNow() error {
	err := uploadBackup(s.store.Snapshot())
	at := time.Now().UTC()
	s.backup = BackupStatus{At: &at}
	if err != nil {
		s.backup.Error = err.Error()
	}
	return err
}

func (s *Service) Backup() error { s.mu.Lock(); defer s.mu.Unlock(); return s.backupNow() }

func (s *Service) SaveSettings(settings Settings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	settings.WebDAVURL = strings.TrimSpace(settings.WebDAVURL)
	if err := validateSettings(settings); err != nil {
		return err
	}
	if err := s.store.Update(func(d *Data) error { d.Settings = settings; return nil }); err != nil {
		return err
	}
	s.backup = BackupStatus{}
	s.autoBackup()
	return nil
}

type RestorePreview struct {
	ID        string `json:"id"`
	Accounts  int    `json:"accounts"`
	WebDAVURL string `json:"webdav_url"`
}

func (s *Service) PrepareRestore() (RestorePreview, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.restore = nil
	s.restoreID = ""
	d, err := downloadBackup(s.store.Snapshot().Settings)
	if err != nil {
		return RestorePreview{}, err
	}
	s.restore = &d
	s.restoreID = uuid.NewString()
	return RestorePreview{ID: s.restoreID, Accounts: len(d.Accounts), WebDAVURL: d.Settings.WebDAVURL}, nil
}

func (s *Service) CancelRestore() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.restore = nil
	s.restoreID = ""
}

func (s *Service) Restore(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.restore == nil || id != s.restoreID {
		return fmt.Errorf("恢复预览已失效，请重新下载")
	}
	if err := s.store.Update(func(d *Data) error { *d = cloneData(*s.restore); return nil }); err != nil {
		return err
	}
	s.restore = nil
	s.restoreID = ""
	s.usageErrors = map[string]string{}
	s.creditErrors = map[string]string{}
	s.backup = BackupStatus{}
	return nil
}

func (s *Service) Rename(id, alias string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, err := findAccount(s.store.Snapshot(), id)
	if err != nil {
		return err
	}
	a.Alias = strings.TrimSpace(alias)
	if err = s.saveAccount(a); err != nil {
		return err
	}
	s.autoBackup()
	return nil
}

func (s *Service) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.store.Update(func(d *Data) error {
		for n, a := range d.Accounts {
			if a.ID == id {
				d.Accounts = append(d.Accounts[:n], d.Accounts[n+1:]...)
				if d.LastSwitchedID == id {
					d.LastSwitchedID = ""
				}
				return nil
			}
		}
		return fmt.Errorf("账号不存在")
	}); err != nil {
		return err
	}
	delete(s.usageErrors, id)
	delete(s.creditErrors, id)
	s.autoBackup()
	return nil
}

func (s *Service) BeginLogin(target string) (*Login, error) {
	s.loginMu.Lock()
	defer s.loginMu.Unlock()
	if s.login != nil {
		return nil, fmt.Errorf("已有登录进行中")
	}
	if target != "" {
		s.mu.Lock()
		_, err := findAccount(s.store.Snapshot(), target)
		s.mu.Unlock()
		if err != nil {
			return nil, err
		}
	}
	l, err := s.oauth.Start(s.ctx)
	if err != nil {
		return nil, err
	}
	s.login = l
	s.loginTarget = target
	return l, nil
}

func (s *Service) CancelLogin(id string) {
	s.loginMu.Lock()
	defer s.loginMu.Unlock()
	if s.login != nil && s.login.ID == id {
		s.login.Cancel()
	}
}
func (s *Service) Shutdown() {
	s.loginMu.Lock()
	defer s.loginMu.Unlock()
	if s.login != nil {
		s.login.Cancel()
	}
}

func (s *Service) WaitLogin(id string) error {
	s.loginMu.Lock()
	l := s.login
	target := s.loginTarget
	s.loginMu.Unlock()
	if l == nil || l.ID != id {
		return fmt.Errorf("登录已结束")
	}
	defer func() {
		s.loginMu.Lock()
		if s.login == l {
			s.login = nil
			s.loginTarget = ""
		}
		s.loginMu.Unlock()
	}()
	t, i, err := s.oauth.Wait(l)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if l.canceled.Load() || s.ctx.Err() != nil {
		return fmt.Errorf("登录已取消")
	}
	if err = s.acceptLogin(target, t, i); err != nil {
		return err
	}
	s.autoBackup()
	return nil
}

func (s *Service) acceptLogin(target string, t Tokens, i Identity) error {
	return s.store.Update(func(d *Data) error {
		if target != "" {
			a, err := findAccount(*d, target)
			if err != nil {
				return err
			}
			if !identityMatches(a, i) {
				return fmt.Errorf("登录身份与目标账号不匹配，请选择原账号及工作区")
			}
		}
		for n, a := range d.Accounts {
			if identityMatches(a, i) {
				d.Accounts[n].Tokens = t
				d.Accounts[n].Email = i.Email
				d.Accounts[n].Plan = i.Plan
				delete(s.usageErrors, a.ID)
				delete(s.creditErrors, a.ID)
				return nil
			}
		}
		d.Accounts = append(d.Accounts, Account{ID: uuid.NewString(), UserID: i.UserID, AccountID: i.AccountID, Email: i.Email, Plan: i.Plan, Tokens: t})
		return nil
	})
}

func (s *Service) prepareAccount(id string, force bool) (Account, error) {
	a, err := findAccount(s.store.Snapshot(), id)
	if err != nil {
		return a, err
	}
	cliAccount, current, err := s.cli.ImportCurrent(a)
	if err != nil {
		return a, err
	}
	if current && !cliAccount.Tokens.LastRefresh.Before(a.Tokens.LastRefresh) && (cliAccount.Tokens.IDToken != a.Tokens.IDToken || cliAccount.Tokens.AccessToken != a.Tokens.AccessToken || cliAccount.Tokens.RefreshToken != a.Tokens.RefreshToken) {
		a = cliAccount
		if err = s.saveAccount(a); err != nil {
			return a, err
		}
		s.autoBackup()
	}
	if !force && time.Until(a.Tokens.ExpiresAt) > time.Minute {
		return a, nil
	}
	t, i, err := s.oauth.Refresh(s.ctx, a.Tokens)
	if err != nil {
		return a, err
	}
	if !identityMatches(a, i) {
		return a, fmt.Errorf("刷新后账号身份改变，请重新登录")
	}
	a.Tokens = t
	a.Email = i.Email
	a.Plan = i.Plan
	if err = s.saveAccount(a); err != nil {
		return a, err
	}
	var cliErr error
	if current {
		cliErr = s.cli.WriteIfUnchanged(cliAccount, t)
	}
	s.autoBackup()
	if cliErr != nil {
		return a, cliErr
	}
	return a, nil
}

func (s *Service) authenticated(id string, request func(Account) error) error {
	a, err := s.prepareAccount(id, false)
	if err != nil {
		return err
	}
	err = request(a)
	var httpErr *HTTPError
	if errors.As(err, &httpErr) && httpErr.Status == http.StatusUnauthorized {
		a, err = s.prepareAccount(id, true)
		if err != nil {
			return err
		}
		return request(a)
	}
	return err
}

func (s *Service) refreshUsage(id string) error {
	var usage *Usage
	err := s.authenticated(id, func(a Account) error { var err error; usage, err = s.backend.Usage(s.ctx, a); return err })
	if err == nil {
		a, e := findAccount(s.store.Snapshot(), id)
		if e != nil {
			return e
		}
		a.Usage = usage
		err = s.saveAccount(a)
	}
	if err != nil {
		s.usageErrors[id] = err.Error()
	} else {
		delete(s.usageErrors, id)
	}
	return err
}

func (s *Service) refreshCredits(id string) error {
	var credits *Credits
	err := s.authenticated(id, func(a Account) error { var err error; credits, err = s.backend.Credits(s.ctx, a); return err })
	if err == nil {
		a, e := findAccount(s.store.Snapshot(), id)
		if e != nil {
			return e
		}
		a.Credits = credits
		err = s.saveAccount(a)
	}
	if err != nil {
		s.creditErrors[id] = err.Error()
	} else {
		delete(s.creditErrors, id)
	}
	return err
}

func (s *Service) RefreshAll() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, a := range s.store.Snapshot().Accounts {
		s.refreshUsage(a.ID)
	}
	return s.state()
}

func (s *Service) RefreshAccount(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.refreshUsage(id)
}
func (s *Service) RefreshCredits(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.refreshCredits(id)
}

func (s *Service) Switch(id string, enableFile bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := findAccount(s.store.Snapshot(), id); err != nil {
		return err
	}
	mode, err := s.cli.StoreMode()
	if err != nil {
		return err
	}
	if mode != "file" {
		if !enableFile {
			return fmt.Errorf("请确认将 CLI 凭据存储改为文件存储")
		}
		if err = s.cli.EnableFileStore(); err != nil {
			return err
		}
	}
	a, err := s.prepareAccount(id, false)
	if err != nil {
		return err
	}
	if err = s.cli.Write(a.Tokens); err != nil {
		return fmt.Errorf("写入 CLI 认证失败: %w", err)
	}
	if err = s.store.Update(func(d *Data) error { d.LastSwitchedID = id; return nil }); err != nil {
		return fmt.Errorf("CLI 已切换，本地记录保存失败: %w", err)
	}
	s.autoBackup()
	return nil
}

func (s *Service) PrepareConsume(id, creditID string) (ResetRequest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, err := findAccount(s.store.Snapshot(), id)
	if err != nil {
		return ResetRequest{}, err
	}
	if a.PendingReset != nil && a.PendingReset.Result == nil {
		if a.PendingReset.CreditID != creditID && a.PendingReset.Started {
			return ResetRequest{}, fmt.Errorf("上次消费结果未确定，请先重试原请求")
		}
		if a.PendingReset.CreditID == creditID {
			return *a.PendingReset, nil
		}
	}
	a.PendingReset = &ResetRequest{ID: uuid.NewString(), CreditID: creditID}
	if err = s.saveAccount(a); err != nil {
		return ResetRequest{}, err
	}
	return *a.PendingReset, nil
}

func (s *Service) Consume(id, requestID string) (ConsumeResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, err := findAccount(s.store.Snapshot(), id)
	if err != nil {
		return ConsumeResult{}, err
	}
	if a.PendingReset == nil || a.PendingReset.ID != requestID {
		return ConsumeResult{}, fmt.Errorf("消费请求已失效")
	}
	if a.PendingReset.Result != nil {
		return *a.PendingReset.Result, nil
	}
	a, err = s.prepareAccount(id, false)
	if err != nil {
		return ConsumeResult{}, err
	}
	a.PendingReset.Started = true
	if err = s.saveAccount(a); err != nil {
		return ConsumeResult{}, err
	}
	var result ConsumeResult
	err = s.authenticated(id, func(account Account) error {
		var e error
		result, e = s.backend.Consume(s.ctx, account, requestID, account.PendingReset.CreditID)
		return e
	})
	if err != nil {
		return result, fmt.Errorf("消费结果未确定，重试将沿用原请求标识: %w", err)
	}
	a, err = findAccount(s.store.Snapshot(), id)
	if err != nil {
		return result, err
	}
	a.PendingReset.Result = &result
	if err = s.saveAccount(a); err != nil {
		return result, fmt.Errorf("服务端已返回 %s，保存消费结果失败: %w", result.Code, err)
	}
	usageErr := s.refreshUsage(id)
	creditErr := s.refreshCredits(id)
	if err = errors.Join(usageErr, creditErr); err != nil {
		result.RefreshError = err.Error()
	}
	return result, nil
}
