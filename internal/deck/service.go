package deck

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"codex-deck/internal/network"

	"github.com/google/uuid"
)

type Service struct {
	HTTP                *network.Router
	authImport          *pendingAuthImport
	generation          uint64
	mu                  sync.Mutex
	refreshMu           sync.Mutex
	autoMu              sync.Mutex
	autoRunMu           sync.Mutex
	autoCancel          context.CancelFunc
	autoPaused          bool
	autoRefreshing      bool
	autoOnce            sync.Once
	autoWorkers         sync.WaitGroup
	autoWake            chan struct{}
	store               *Store
	cli                 CLI
	cliProxyMu          sync.Mutex
	oauth               OAuth
	backend             Backend
	ctx                 context.Context
	usageErrors         map[string]string
	creditErrors        map[string]string
	usageTries          map[string]time.Time
	cliIdentity         Identity
	importError         string
	notifier            func(string) error
	notificationWorkers sync.WaitGroup
	desktopError        string
	watchError          string
	watchWorkers        sync.WaitGroup
	onChange            func(State)
	backup              BackupStatus
	backupIO            sync.Mutex
	backupOnce          sync.Once
	backupWorkers       sync.WaitGroup
	backupWake          chan struct{}
	backupPending       bool
	backupCancel        context.CancelFunc
	restoreFiles        []BackupFile
	restoreListSource   Settings
	restoreFile         BackupFile
	restoreBody         []byte
	restoreSource       Settings
	cancel              context.CancelFunc
	restore             *Data
	restoreID           string
	loginMu             sync.Mutex
	login               *Login
	loginTarget         string
}

func NewService(ctx context.Context, path, cliHome string) (*Service, error) {
	store, err := OpenStore(path)
	if err != nil {
		return nil, err
	}
	config := network.Config{}
	if saved := store.Snapshot().Network; saved != nil {
		config = *saved
	}
	router, err := network.New(config)
	if err != nil {
		return nil, err
	}
	ctx = network.WithRouter(ctx, router)
	ctx, cancel := context.WithCancel(ctx)
	client := &http.Client{Transport: router, Timeout: 30 * time.Second}
	s := &Service{cancel: cancel, autoWake: make(chan struct{}, 1), backupWake: make(chan struct{}, 1), store: store, cli: CLI{Home: cliHome}, oauth: OAuth{Client: client, Issuer: "https://auth.openai.com"}, backend: Backend{Client: client, BaseURL: "https://chatgpt.com/backend-api"}, ctx: ctx, usageErrors: map[string]string{}, creditErrors: map[string]string{}, usageTries: map[string]time.Time{}}
	s.HTTP = router
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
			_, err = s.syncAccountCredentials(existing, *local)
			break
		}
		// 自动刷新不重新导入本次会话中已删除或被整体恢复移除的账号。
		if automatic && !managed && i.UserID == s.cliIdentity.UserID && i.AccountID == s.cliIdentity.AccountID {
			return
		}
		if !managed {
			err = s.acceptLogin("", local.Tokens, i)
			if err == nil {
				s.autoBackup()
			}
		}
		if err == nil {
			s.cliIdentity = i
		}
	} else if err == nil {
		s.cliIdentity = Identity{}
	}
	if err != nil {
		s.importError = fmt.Sprintf("同步 CLI 账号失败: %v", err)
	}
}

// 调用者持有 mu。时间相同或缺失时，CLI 无法证明不同凭据更新。
func (s *Service) syncAccountCredentials(account, local Account) (Account, error) {
	changed := !sameCredentials(account.Tokens, local.Tokens)
	if changed {
		if account.Tokens.LastRefresh.IsZero() || local.Tokens.LastRefresh.IsZero() || !local.Tokens.LastRefresh.After(account.Tokens.LastRefresh) {
			return account, nil
		}
		local.Tokens.OAuthResponse = account.Tokens.OAuthResponse
		account.Tokens = local.Tokens
		account.Email = local.Email
		account.Plan = local.Plan
		account.AuthError = ""
	} else {
		if !local.Tokens.LastRefresh.After(account.Tokens.LastRefresh) {
			return account, nil
		}
		account.Tokens.LastRefresh = local.Tokens.LastRefresh
	}
	if err := s.saveAccount(account); err != nil {
		return account, err
	}
	if changed {
		delete(s.usageErrors, account.ID)
		delete(s.creditErrors, account.ID)
	}
	s.autoBackup()
	return account, nil
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
	state := State{DesktopError: s.desktopError, Preferences: d.Preferences, Accounts: []AccountView{}, Settings: d.Settings, Backup: s.backup, CLIError: s.importError, NextRefreshAt: &nextRefresh, AutoRefreshing: s.autoRefreshing}
	state.Network = network.Config{Mode: "environment"}
	if d.Network != nil {
		state.Network = *d.Network
		if state.Network.Mode == "" {
			state.Network.Mode = "environment"
		}
	}
	if s.watchError != "" {
		state.CLIError = strings.TrimSpace(state.CLIError + " " + s.watchError)
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
	ID             string           `json:"id"`
	Accounts       int              `json:"accounts"`
	WebDAVURL      string           `json:"webdav_url"`
	ModifiedAt     *time.Time       `json:"modified_at" ts_type:"string | null"`
	Added          []PreviewAccount `json:"added"`
	Removed        []PreviewAccount `json:"removed"`
	RemovesCurrent bool             `json:"removes_current"`
	File           BackupFile       `json:"file"`
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

func (s *Service) ListBackups() ([]BackupFile, error) {
	s.mu.Lock()
	settings := s.store.Snapshot().Settings
	generation := s.generation
	s.mu.Unlock()
	files, err := listBackupFiles(s.ctx, settings)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.store.Snapshot().Settings != settings || s.generation != generation {
		return nil, fmt.Errorf("设置或本地数据已改变，请重新读取备份列表")
	}
	s.restoreFiles = files
	s.restoreListSource = settings
	return files, nil
}

func (s *Service) PrepareRestore(filename string) (RestorePreview, error) {
	s.CancelAutomatic()
	s.mu.Lock()
	settings := s.store.Snapshot().Settings
	generation := s.generation
	var file BackupFile
	for _, candidate := range s.restoreFiles {
		if candidate.ID == filename {
			file = candidate
			break
		}
	}
	if file.ID == "" || settings != s.restoreListSource {
		s.mu.Unlock()
		return RestorePreview{}, fmt.Errorf("备份列表已失效，请重新选择文件")
	}
	s.restore = nil
	s.restoreID = ""
	s.restoreBody = nil
	s.mu.Unlock()
	d, metadata, body, err := readSnapshot(s.ctx, settings, filename)
	if err != nil {
		return RestorePreview{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	local := s.store.Snapshot()
	if local.Settings != settings || s.generation != generation {
		return RestorePreview{}, fmt.Errorf("设置或本地数据已改变，请重新下载备份")
	}
	if metadata.Hostname != "" {
		file.Hostname = metadata.Hostname
	}
	if !metadata.CreatedAt.IsZero() {
		file.CreatedAt = &metadata.CreatedAt
	}
	if metadata.ModifiedAt != nil {
		file.ModifiedAt = metadata.ModifiedAt
	}
	s.restore = &d
	s.restoreFile = file
	s.restoreBody = body
	s.restoreSource = settings
	s.restoreID = uuid.NewString()
	preview := RestorePreview{ID: s.restoreID, File: file, Accounts: len(d.Accounts), WebDAVURL: d.Settings.WebDAVURL, ModifiedAt: file.ModifiedAt, Added: accountDifference(d.Accounts, local.Accounts), Removed: accountDifference(local.Accounts, d.Accounts)}
	for _, view := range s.state().Accounts {
		if !view.Current {
			continue
		}
		account, err := findAccount(local, view.ID)
		if err != nil {
			return RestorePreview{}, err
		}
		preview.RemovesCurrent = len(accountDifference([]Account{account}, d.Accounts)) > 0
		break
	}
	return preview, nil
}

func (s *Service) CancelRestore() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.restore = nil
	s.restoreID = ""
	s.restoreBody = nil
}

func (s *Service) Restore(id string) error {
	s.CancelAutomatic()
	s.stopBackup()
	s.backupIO.Lock()
	defer s.backupIO.Unlock()
	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.restore == nil || id != s.restoreID {
		return fmt.Errorf("恢复预览已失效，请重新下载")
	}
	source, filename, body := s.restoreSource, s.restoreFile.ID, s.restoreBody
	s.mu.Unlock()
	_, _, latest, err := readSnapshot(s.ctx, source, filename)
	s.mu.Lock()
	if err != nil {
		return err
	}
	if !bytes.Equal(latest, body) {
		return fmt.Errorf("远端备份已改变，请重新下载预览")
	}
	if s.restore == nil || id != s.restoreID || s.store.Snapshot().Settings != source {
		return fmt.Errorf("恢复预览已失效，请重新下载")
	}
	current, cliErr := s.cli.ReadAccount()
	if err := s.store.Update(func(d *Data) error {
		localNetwork := d.Network
		*d = cloneData(*s.restore)
		d.Network = localNetwork
		return nil
	}); err != nil {
		return err
	}
	s.cliIdentity = Identity{}
	s.importError = ""
	if cliErr != nil {
		s.importError = fmt.Sprintf("同步 CLI 账号失败: %v", cliErr)
	} else if current != nil {
		s.cliIdentity = Identity{UserID: current.UserID, AccountID: current.AccountID, Email: current.Email, Plan: current.Plan}
	}
	s.generation++
	s.restore = nil
	s.restoreID = ""
	s.restoreBody = nil
	s.restoreFiles = nil
	s.usageErrors = map[string]string{}
	s.creditErrors = map[string]string{}
	s.usageTries = map[string]time.Time{}
	s.backup = BackupStatus{}
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
	mode, err := s.cli.StoreMode()
	if err != nil {
		return fmt.Errorf("核对当前账号失败: %w", err)
	}
	if err := s.store.Update(func(d *Data) error {
		for n, a := range d.Accounts {
			if a.ID == id {
				if mode == "file" {
					_, current, err := s.cli.Read()
					if err != nil {
						return fmt.Errorf("核对当前账号失败: %w", err)
					}
					if identityMatches(a, current) {
						return fmt.Errorf("不能删除当前账号，请先切换到其他账号")
					}
				}
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
	s.autoWorkers.Wait()
	s.stopBackup()
	s.backupWorkers.Wait()
	s.watchWorkers.Wait()
	s.notificationWorkers.Wait()
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
	if current {
		a, err = s.syncAccountCredentials(a, cliAccount)
		if err != nil {
			return a, err
		}
	}
	writeCLI := current && sameCredentials(cliAccount.Tokens, a.Tokens)
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
		return a, s.authenticationFailed(id, "刷新后账号身份改变，请重新登录")
	}
	a.Tokens = t
	a.Email = i.Email
	a.Plan = i.Plan
	if err = s.saveAccount(a); err != nil {
		return a, err
	}
	var cliErr error
	if writeCLI {
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
		err = request(a)
		if errors.As(err, &httpErr) && httpErr.Status == http.StatusUnauthorized {
			return s.authenticationFailed(id, "刷新后认证仍未通过，请重新登录")
		}
		return err
	}
	return err
}

func (s *Service) refreshUsage(id string) error { return s.refreshUsageContext(s.ctx, id) }

func (s *Service) refreshUsageContext(ctx context.Context, id string) error {
	generation := s.generation
	s.usageTries[id] = time.Now()
	var usage *Usage
	err := s.authenticated(id, func(a Account) error {
		s.mu.Unlock()
		var err error
		usage, err = s.backend.Usage(ctx, a)
		s.mu.Lock()
		if ctx.Err() != nil {
			return ctx.Err()
		}
		current, e := findAccount(s.store.Snapshot(), id)
		if e != nil || s.generation != generation || current.Tokens.AccessToken != a.Tokens.AccessToken {
			return ErrAccountChanged
		}
		return err
	})
	if errors.Is(err, ErrAccountChanged) {
		return err
	}
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
		message := s.lowQuotaNotice(&a, s.store.Snapshot().Preferences)
		err = s.saveAccount(a)
		if err == nil {
			s.sendQuotaNotice(message)
		}
	}
	if err != nil {
		s.usageErrors[id] = err.Error()
	} else {
		delete(s.usageErrors, id)
	}
	return err
}

func (s *Service) refreshCredits(id string) error {
	generation := s.generation
	var credits *Credits
	err := s.authenticated(id, func(a Account) error {
		s.mu.Unlock()
		var err error
		credits, err = s.backend.Credits(s.ctx, a)
		s.mu.Lock()
		current, e := findAccount(s.store.Snapshot(), id)
		if e != nil || s.generation != generation || current.Tokens.AccessToken != a.Tokens.AccessToken {
			return ErrAccountChanged
		}
		return err
	})
	if errors.Is(err, ErrAccountChanged) {
		return err
	}
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
	if a.PendingReset == nil || a.PendingReset.ID != requestID {
		return ConsumeResult{}, ErrAccountChanged
	}
	generation := s.generation
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
	if s.generation != generation || a.PendingReset == nil || a.PendingReset.ID != requestID {
		return result, fmt.Errorf("服务端已返回 %s，本地数据已改变，未写入消费结果", result.Code)
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

var ErrAccountChanged = errors.New("操作期间账号数据已改变，请重试")

func (s *Service) authenticationFailed(id, message string) error {
	a, err := findAccount(s.store.Snapshot(), id)
	if err != nil {
		return err
	}
	a.AuthError = message
	err = s.saveAccount(a)
	if err == nil {
		s.autoBackup()
	}
	return errors.Join(&AuthenticationError{Message: message}, err)
}
