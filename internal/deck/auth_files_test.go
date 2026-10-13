package deck

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeTestAuth(t *testing.T, tokens Tokens) string {
	t.Helper()
	data, err := encodeCLIAuth(tokens)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "auth.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestAuthImportAddsAccountWithoutWritingCLI(t *testing.T) {
	s := testService(t)
	existing := testAccount(t)
	addTestAccount(t, s, existing)
	imported := testAccount(t)
	imported.UserID = "other-user"
	imported.AccountID = "other-workspace"
	imported.Tokens.IDToken = testToken(t, imported.UserID, imported.AccountID, time.Now().Add(time.Hour))
	imported.Tokens.AccessToken = imported.Tokens.IDToken
	imported.Tokens.AccountID = imported.AccountID
	path := writeTestAuth(t, imported.Tokens)
	if err := s.cli.Write(existing.Tokens); err != nil {
		t.Fatal(err)
	}
	cliPath := filepath.Join(s.cli.Home, "auth.json")
	before, err := os.ReadFile(cliPath)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := s.PrepareAuthImport(path)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Existing || preview.AccountID != imported.AccountID || len(s.GetState().Accounts) != 1 {
		t.Fatal("导入预览改写账号或身份错误")
	}
	view, err := json.Marshal(preview)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(view), imported.Tokens.RefreshToken) || strings.Contains(string(view), "access_token") {
		t.Fatal("预览泄露凭据")
	}
	if err := s.ConfirmAuthImport(preview.ID); err != nil {
		t.Fatal(err)
	}
	accounts := s.store.Snapshot().Accounts
	if len(accounts) != 2 || accounts[0].ID != existing.ID || accounts[1].AccountID != imported.AccountID {
		t.Fatal("账号添加错误")
	}
	after, err := os.ReadFile(cliPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("导入改写 CLI 凭据")
	}
}

func TestAuthImportUpdatesCredentialsAndPreservesAccount(t *testing.T) {
	s := testService(t)
	account := testAccount(t)
	account.Alias = "我的别名"
	account.AuthError = "请重新登录"
	account.Usage = &Usage{QueriedAt: time.Now()}
	addTestAccount(t, s, account)
	newTokens := account.Tokens
	newTokens.LastRefresh = account.Tokens.LastRefresh.Add(time.Minute)
	newTokens.RefreshToken = "new-refresh"
	preview, err := s.PrepareAuthImport(writeTestAuth(t, newTokens))
	if err != nil {
		t.Fatal(err)
	}
	if !preview.Existing || preview.Alias != account.Alias {
		t.Fatal("未识别已有账号")
	}
	if err := s.ConfirmAuthImport(preview.ID); err != nil {
		t.Fatal(err)
	}
	accounts := s.store.Snapshot().Accounts
	if len(accounts) != 1 || accounts[0].ID != account.ID || accounts[0].Alias != account.Alias || accounts[0].Usage == nil || accounts[0].AuthError != "" || accounts[0].Tokens.RefreshToken != "new-refresh" {
		t.Fatal("覆盖账号时丢失展示数据或凭据未更新")
	}
}

func TestAuthImportRejectsInvalidAndOlderCredentials(t *testing.T) {
	s := testService(t)
	account := testAccount(t)
	addTestAccount(t, s, account)
	before, err := os.ReadFile(s.store.path)
	if err != nil {
		t.Fatal(err)
	}
	valid, err := encodeCLIAuth(account.Tokens)
	if err != nil {
		t.Fatal(err)
	}
	var mismatched cliAuth
	if err := json.Unmarshal(valid, &mismatched); err != nil {
		t.Fatal(err)
	}
	mismatched.Tokens.AccountID = "wrong-workspace"
	wrong, err := json.Marshal(mismatched)
	if err != nil {
		t.Fatal(err)
	}
	old := account.Tokens
	old.LastRefresh = old.LastRefresh.Add(-time.Hour)
	oldData, err := encodeCLIAuth(old)
	if err != nil {
		t.Fatal(err)
	}
	missing := account.Tokens
	missing.RefreshToken = ""
	missingData, err := encodeCLIAuth(missing)
	if err != nil {
		t.Fatal(err)
	}
	for _, data := range [][]byte{[]byte(`{"tokens":`), []byte(`{"auth_mode":"apikey","OPENAI_API_KEY":"local-key"}`), []byte(`null`), wrong, oldData, missingData} {
		path := filepath.Join(t.TempDir(), "auth.json")
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := s.PrepareAuthImport(path); err == nil {
			t.Fatal("接受无效或旧凭据")
		}
	}
	after, err := os.ReadFile(s.store.path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("导入失败改写数据")
	}
}

func TestAuthImportRechecksCredentialsAndCancellation(t *testing.T) {
	s := testService(t)
	account := testAccount(t)
	addTestAccount(t, s, account)
	path := writeTestAuth(t, account.Tokens)
	preview, err := s.PrepareAuthImport(path)
	if err != nil {
		t.Fatal(err)
	}
	s.CancelAuthImport()
	if err := s.ConfirmAuthImport(preview.ID); err == nil {
		t.Fatal("取消后仍能导入")
	}
	preview, err = s.PrepareAuthImport(path)
	if err != nil {
		t.Fatal(err)
	}
	account.Tokens.LastRefresh = account.Tokens.LastRefresh.Add(time.Hour)
	account.Tokens.RefreshToken = "latest-refresh"
	if err := s.saveAccount(account); err != nil {
		t.Fatal(err)
	}
	if err := s.ConfirmAuthImport(preview.ID); err == nil {
		t.Fatal("确认时覆盖更新凭据")
	}
	if s.store.Snapshot().Accounts[0].Tokens.RefreshToken != "latest-refresh" {
		t.Fatal("凭据被回退")
	}
}

func TestAuthImportRejectsPreviewAfterRestore(t *testing.T) {
	s := testService(t)
	account := testAccount(t)
	preview, err := s.PrepareAuthImport(writeTestAuth(t, account.Tokens))
	if err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	s.generation++
	s.mu.Unlock()
	if err := s.ConfirmAuthImport(preview.ID); err == nil {
		t.Fatal("恢复后使用旧导入预览")
	}
	if len(s.store.Snapshot().Accounts) != 0 {
		t.Fatal("旧导入改写恢复数据")
	}
}
