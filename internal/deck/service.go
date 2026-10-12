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
	mu            sync.Mutex
	refreshMu     sync.Mutex
	autoMu        sync.Mutex
	autoCancel    context.CancelFunc
	store         *Store
	cli           CLI
	oauth         OAuth
	backend       Backend
	ctx           context.Context
	usageErrors   map[string]string
	creditErrors  map[string]string
	usageTries    map[string]time.Time
	cliIdentity   Identity
	importError   string
	onChange      func(State)
	backup        BackupStatus
	backupIO      sync.Mutex
	backupOnce    sync.Once
	backupWorkers sync.WaitGroup
	backupWake    chan struct{}
	backupPending bool
	backupCancel  context.CancelFunc
	conflictETag  string
	restoreMeta   remoteBackup
	restoreSource Settings
	cancel        context.CancelFunc
	restore       *Data
	restoreID     string
	loginMu       sync.Mutex
	login         *Login
	loginTarget   string
}

func NewService(ctx context.Context, path, cliHome string) (*Service, error) {
	store, err := OpenStore(path)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	client := &http.Client{Timeout: 30 * time.Second}
	s := &Service{cancel: cancel, backupWake: make(chan struct{}, 1), store: store, cli: CLI{Home: cliHome}, oauth: OAuth{Client: client, Issuer: "https://auth.openai.com"}, backend: Backend{Client: client, BaseURL: "https://chatgpt.com/backend-api"}, ctx: ctx, usageErrors: map[string]string{}, creditErrors: map[string]string{}, usageTries: map[string]time.Time{}}
	s.mu.Lock()
	s.syncCLIAccount(false)
	s.mu.Unlock()
	return s, nil
}

func (s *Service) syncCLIAccount(automatic bool) {
	s.importError = ""
	local, err := s.cli.ReadAccount()
	if err == nil && local != nil {
		i := Identity{UserID: local.UserID, AccountID: local.AccountID, Email: local.Email, Plan: local.Plan}
		managed := false
		for _, existing := range s.store.Snapshot().Accounts {
			if !identityMatches(existing, i) {
				continue
			}
			managed = true
			if local.Tokens.LastRefresh.Before(existing.Tokens.LastRefresh) {
				s.cliIdentity = i
				return
			}
			if local.Tokens.IDToken == existing.Tokens.IDToken && local.Tokens.AccessToken == existing.Tokens.AccessToken && local.Tokens.RefreshToken == existing.Tokens.RefreshToken && local.Tokens.LastRefresh.Equal(existing.Tokens.LastRefresh) && local.Email == existing.Email && local.Plan == existing.Plan {
				s.cliIdentity = i
				return
			}
			local.Tokens.OAuthResponse = existing.Tokens.OAuthResponse
			break
		}
		// 自动刷新不重新导入本次会话中已删除或被整体恢复移除的账号。
		if automatic && !managed && i.UserID == s.cliIdentity.UserID && i.AccountID == s.cliIdentity.AccountID {
			return
		}
		err = s.acceptLogin("", local.Tokens, i)
		if err == nil {
			s.cliIdentity = i
			s.autoBackup()
		}
	} else if err == nil {
		s.cliIdentity = Identity{}
	}
	if err != nil {
		s.importError = fmt.Sprintf("同步 CLI 账号失败: %v", err)
	}
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
	nextRefresh := time.Now().Add(autoRefreshInterval)
	for _, account := range d.Accounts {
		if account.AuthError != "" {
			continue
		}
		next := nextUsageRefresh(account, s.usageTries[account.ID])
		if next.IsZero() {
			next = time.Now()
		}
		if next.Before(nextRefresh) {
			nextRefresh = next
		}
	}
	state := State{Accounts: []AccountView{}, Settings: d.Settings, Backup: s.backup, CLIError: s.importError, NextRefreshAt: &nextRefresh}
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
		state.Accounts = append(state.Accounts, AccountView{ID: a.ID, AccountID: a.AccountID, Email: a.Email, Alias: a.Alias, Plan: plan, AuthError: a.AuthError, Current: mode == "file" && identityMatches(a, current), Usage: a.Usage, Credits: a.Credits, UsageError: s.usageErrors[a.ID], CreditsError: s.creditErrors[a.ID], PendingReset: a.PendingReset})
	}
	return state
}

func (s *Service) GetState() State { s.mu.Lock(); defer s.mu.Unlock(); return s.state() }

type PreviewAccount struct {
	Email     string `json:"email"`
	Alias     string `json:"alias"`
	AccountID string `json:"account_id"`
}
type RestorePreview struct {
	ID         string           `json:"id"`
	Accounts   int              `json:"accounts"`
	WebDAVURL  string           `json:"webdav_url"`
	ModifiedAt *time.Time       `json:"modified_at"`
	Added      []PreviewAccount `json:"added"`
	Removed    []PreviewAccount `json:"removed"`
}

func accountDifference(left, right []Account) []PreviewAccount {
	result := []PreviewAccount{}
	for _, a := range left {
		found := false
		for _, b := range right {
			if a.UserID == b.UserID && a.AccountID == b.AccountID {
				found = true
				break
			}
		}
		if !found {
			result = append(result, PreviewAccount{Email: a.Email, Alias: a.Alias, AccountID: a.AccountID})
		}
	}
	return result
}

func (s *Service) PrepareRestore() (RestorePreview, error) {
	s.CancelAutomatic()
	settings := s.store.Snapshot().Settings
	d, meta, err := readBackup(s.ctx, settings)
	if err != nil {
		return RestorePreview{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	local := s.store.Snapshot()
	if local.Settings != settings {
		return RestorePreview{}, fmt.Errorf("设置已改变，请重新下载备份")
	}
	s.restore = &d
	s.restoreMeta = meta
	s.restoreSource = settings
	s.restoreID = uuid.NewString()
	return RestorePreview{ID: s.restoreID, Accounts: len(d.Accounts), WebDAVURL: d.Settings.WebDAVURL, ModifiedAt: meta.ModifiedAt, Added: accountDifference(d.Accounts, local.Accounts), Removed: accountDifference(local.Accounts, d.Accounts)}, nil
}

func (s *Service) CancelRestore() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.restore = nil
	s.restoreID = ""
}

func (s *Service) Restore(id string) error {
	s.CancelAutomatic()
	s.stopBackup()
	s.backupIO.Lock()
	defer s.backupIO.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.restore == nil || id != s.restoreID {
		return fmt.Errorf("恢复预览已失效，请重新下载")
	}
	source, tag := s.restoreSource, s.restoreMeta.ETag
	s.mu.Unlock()
	meta, exists, err := backupMetadata(s.ctx, source)
	s.mu.Lock()
	if err != nil {
		return err
	}
	if !exists || meta.ETag != tag {
		return fmt.Errorf("远端备份已改变，请重新下载预览")
	}
	if s.restore == nil || id != s.restoreID || s.store.Snapshot().Settings != source {
		return fmt.Errorf("恢复预览已失效，请重新下载")
	}
	if err := s.store.Update(func(d *Data) error {
		*d = cloneData(*s.restore)
		d.Sync = BackupSync{}
		if d.Settings.WebDAVURL == source.WebDAVURL && d.Settings.WebDAVUsername == source.WebDAVUsername {
			d.Sync = BackupSync{URL: source.WebDAVURL, Username: source.WebDAVUsername, ETag: tag}
		}
		return nil
	}); err != nil {
		return err
	}
	s.restore = nil
	s.restoreID = ""
	s.usageErrors = map[string]string{}
	s.creditErrors = map[string]string{}
	s.usageTries = map[string]time.Time{}
	s.backup = BackupStatus{}
	s.conflictETag = ""
	s.backupPending = false
	return nil
}

func (s *Service) Rename(id, alias string) error {
	s.CancelAutomatic()
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
	s.CancelAutomatic()
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
	delete(s.usageTries, id)
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
	s.cancel()
	s.CancelAutomatic()
	s.stopBackup()
	s.backupWorkers.Wait()
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
				d.Accounts[n].AuthError = ""
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

// 调用者持有 mu。令牌刷新串行执行，但等待和 HTTP 不占用状态锁。
func (s *Service) prepareAccount(id string, force bool) (Account, error) {
	before, _ := findAccount(s.store.Snapshot(), id)
	s.mu.Unlock()
	s.refreshMu.Lock()
	s.mu.Lock()
	defer s.refreshMu.Unlock()
	latest, err := findAccount(s.store.Snapshot(), id)
	if err != nil {
		return latest, err
	}
	if latest.Tokens.AccessToken != before.Tokens.AccessToken {
		force = false
	}
	return s.prepareAccountLocked(id, force)
}

func (s *Service) prepareAccountLocked(id string, force bool) (Account, error) {
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
	if a.AuthError != "" {
		return a, &AuthenticationError{Message: a.AuthError}
	}
	if !force && time.Until(a.Tokens.ExpiresAt) > time.Minute {
		return a, nil
	}
	s.mu.Unlock()
	t, i, err := s.oauth.Refresh(s.ctx, a.Tokens)
	s.mu.Lock()
	latest, latestErr := findAccount(s.store.Snapshot(), id)
	if latestErr != nil {
		return latest, latestErr
	}
	if latest.Tokens.RefreshToken != a.Tokens.RefreshToken || latest.Tokens.AccessToken != a.Tokens.AccessToken {
		return latest, nil
	}
	a = latest
	if err != nil {
		var authErr *AuthenticationError
		if errors.As(err, &authErr) {
			a.AuthError = authErr.Error()
			if saveErr := s.saveAccount(a); saveErr != nil {
				return a, errors.Join(err, saveErr)
			}
			s.autoBackup()
		}
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

func (s *Service) refreshUsage(id string) error { return s.refreshUsageContext(s.ctx, id) }

func (s *Service) refreshUsageContext(ctx context.Context, id string) error {
	s.usageTries[id] = time.Now()
	var usage *Usage
	err := s.authenticated(id, func(a Account) error {
		s.mu.Unlock()
		var err error
		usage, err = s.backend.Usage(ctx, a)
		s.mu.Lock()
		return err
	})
	if ctx.Err() != nil {
		delete(s.usageTries, id)
		return ctx.Err()
	}
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
	err := s.authenticated(id, func(a Account) error {
		s.mu.Unlock()
		var err error
		credits, err = s.backend.Credits(s.ctx, a)
		s.mu.Lock()
		return err
	})
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
	s.CancelAutomatic()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.syncCLIAccount(false)
	for _, a := range s.store.Snapshot().Accounts {
		s.refreshUsage(a.ID)
	}
	return s.state()
}

func (s *Service) RefreshAccount(id string) error {
	s.CancelAutomatic()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.syncCLIAccount(false)
	return s.refreshUsage(id)
}
func (s *Service) RefreshCredits(id string) error {
	s.CancelAutomatic()
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.refreshCredits(id)
}

func (s *Service) Switch(id string, enableFile bool) error {
	s.CancelAutomatic()
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
	s.importError = ""
	s.cliIdentity = Identity{UserID: a.UserID, AccountID: a.AccountID, Email: a.Email, Plan: a.Plan}
	s.autoBackup()
	return nil
}

func (s *Service) PrepareConsume(id, creditID string) (ResetRequest, error) {
	s.CancelAutomatic()
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
	s.CancelAutomatic()
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
		s.mu.Unlock()
		result, e = s.backend.Consume(s.ctx, account, requestID, account.PendingReset.CreditID)
		s.mu.Lock()
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

func (s *Service) SetOnChange(callback func(State)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onChange = callback
}
func (s *Service) publish() {
	if s.onChange != nil {
		s.onChange(s.state())
	}
}
