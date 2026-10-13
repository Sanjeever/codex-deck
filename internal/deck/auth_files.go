package deck

import (
	"fmt"
	"os"
	"strings"

	"github.com/google/uuid"
)

type AuthImportPreview struct {
	ID string `json:"id"`
	PreviewAccount
	Existing bool `json:"existing"`
}

type pendingAuthImport struct {
	id         string
	account    Account
	existingID string
	generation uint64
}

func readAuthAccount(path string) (Account, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Account{}, fmt.Errorf("读取认证文件失败: %w", err)
	}
	auth, identity, err := decodeCLIAuth(data)
	if err != nil {
		return Account{}, err
	}
	if auth == nil || auth.Tokens == nil || identity.UserID == "" || (auth.APIKey != nil && *auth.APIKey != "") {
		return Account{}, fmt.Errorf("仅支持 ChatGPT 账号的 auth.json")
	}
	if strings.TrimSpace(auth.Tokens.RefreshToken) == "" {
		return Account{}, fmt.Errorf("认证文件缺少刷新令牌")
	}
	return accountFromCLI(Account{UserID: identity.UserID, AccountID: identity.AccountID}, auth, identity)
}

func rejectOlderAuth(imported, existing Tokens) error {
	if !imported.LastRefresh.IsZero() && !existing.LastRefresh.IsZero() && imported.LastRefresh.Before(existing.LastRefresh) {
		return fmt.Errorf("导入凭据早于已保存凭据，请选择更新的 auth.json")
	}
	return nil
}

func (s *Service) PrepareAuthImport(path string) (AuthImportPreview, error) {
	s.CancelAutomatic()
	s.mu.Lock()
	s.authImport = nil
	s.mu.Unlock()
	account, err := readAuthAccount(path)
	if err != nil {
		return AuthImportPreview{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	pending := &pendingAuthImport{id: uuid.NewString(), account: account, generation: s.generation}
	preview := AuthImportPreview{ID: pending.id, PreviewAccount: PreviewAccount{Email: account.Email, AccountID: account.AccountID}}
	for _, existing := range s.store.Snapshot().Accounts {
		if existing.UserID != account.UserID || existing.AccountID != account.AccountID {
			continue
		}
		if err := rejectOlderAuth(account.Tokens, existing.Tokens); err != nil {
			return AuthImportPreview{}, err
		}
		pending.existingID = existing.ID
		preview.Existing = true
		preview.Alias = existing.Alias
		break
	}
	s.authImport = pending
	return preview, nil
}

func (s *Service) CancelAuthImport() { s.mu.Lock(); s.authImport = nil; s.mu.Unlock() }

func (s *Service) ConfirmAuthImport(id string) error {
	s.CancelAutomatic()
	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	pending := s.authImport
	if pending == nil || pending.id != id || pending.generation != s.generation {
		return fmt.Errorf("导入预览已失效，请重新选择文件")
	}
	account := pending.account
	identity := Identity{UserID: account.UserID, AccountID: account.AccountID, Email: account.Email, Plan: account.Plan}
	found := false
	for _, existing := range s.store.Snapshot().Accounts {
		if !identityMatches(existing, identity) {
			continue
		}
		if existing.ID != pending.existingID {
			return fmt.Errorf("账号列表已变化，请重新导入")
		}
		if err := rejectOlderAuth(account.Tokens, existing.Tokens); err != nil {
			return err
		}
		account.Tokens.OAuthResponse = existing.Tokens.OAuthResponse
		found = true
		break
	}
	if pending.existingID != "" && !found {
		return fmt.Errorf("账号已移除，请重新导入")
	}
	if err := s.acceptLogin(pending.existingID, account.Tokens, identity); err != nil {
		return err
	}
	s.authImport = nil
	s.autoBackup()
	s.WakeAutomatic()
	return nil
}
