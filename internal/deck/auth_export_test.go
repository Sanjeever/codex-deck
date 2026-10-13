package deck

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAuthExportRoundTripAndPrivateFields(t *testing.T) {
	s := testService(t)
	account := testAccount(t)
	account.Alias = "私有别名"
	account.Usage = &Usage{QueriedAt: time.Now()}
	addTestAccount(t, s, account)
	path := filepath.Join(t.TempDir(), "auth.json")
	if err := s.ExportAuth(account.ID, path, false); err != nil {
		t.Fatal(err)
	}
	imported, err := readAuthAccount(path)
	if err != nil {
		t.Fatal(err)
	}
	if imported.UserID != account.UserID || imported.AccountID != account.AccountID || imported.Tokens.RefreshToken != account.Tokens.RefreshToken {
		t.Fatal("导出无法读取为相同账号")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	for key := range fields {
		if key != "auth_mode" && key != "OPENAI_API_KEY" && key != "tokens" && key != "last_refresh" {
			t.Fatalf("导出包含应用字段 %s", key)
		}
	}
	if _, err := os.Stat(filepath.Join(s.cli.Home, "auth.json")); !os.IsNotExist(err) {
		t.Fatal("普通导出改写 CLI")
	}
}

func TestAuthExportReadsLatestCurrentCredentials(t *testing.T) {
	s := testService(t)
	account := testAccount(t)
	addTestAccount(t, s, account)
	latest := account.Tokens
	latest.RefreshToken = "latest-cli-refresh"
	latest.LastRefresh = latest.LastRefresh.Add(time.Minute)
	if err := s.cli.Write(latest); err != nil {
		t.Fatal(err)
	}
	cliPath := filepath.Join(s.cli.Home, "auth.json")
	before, err := os.ReadFile(cliPath)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "auth.json")
	if err := s.ExportAuth(account.ID, path, false); err != nil {
		t.Fatal(err)
	}
	imported, err := readAuthAccount(path)
	if err != nil {
		t.Fatal(err)
	}
	if imported.Tokens.RefreshToken != latest.RefreshToken || s.store.Snapshot().Accounts[0].Tokens.RefreshToken != latest.RefreshToken {
		t.Fatal("导出没有同步 CLI 最新凭据")
	}
	after, err := os.ReadFile(cliPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("导出同步改写 CLI")
	}
}

func TestAuthExportRequiresCLIConfirmation(t *testing.T) {
	s := testService(t)
	account := testAccount(t)
	addTestAccount(t, s, account)
	old := account.Tokens
	old.AccountID = "other-workspace"
	old.IDToken = testToken(t, account.UserID, old.AccountID, time.Now().Add(time.Hour))
	old.AccessToken = old.IDToken
	if err := s.cli.Write(old); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(s.cli.Home, "auth.json")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ExportAuth(account.ID, path, false); err == nil {
		t.Fatal("未确认就覆盖 CLI")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("未确认时改写 CLI")
	}
	if err := s.ExportAuth(account.ID, path, true); err != nil {
		t.Fatal(err)
	}
	state := s.GetState()
	if !state.Accounts[0].Current || s.store.Snapshot().LastSwitchedID != account.ID {
		t.Fatal("确认覆盖后当前账号状态错误")
	}
}

func TestAuthExportProtectsConfigurationAndInvalidAccounts(t *testing.T) {
	s := testService(t)
	account := testAccount(t)
	addTestAccount(t, s, account)
	before, err := os.ReadFile(s.store.path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ExportAuth(account.ID, s.store.path, true); err == nil {
		t.Fatal("允许覆盖应用配置")
	}
	after, err := os.ReadFile(s.store.path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("破坏应用配置")
	}
	path := filepath.Join(t.TempDir(), "auth.json")
	if err := s.ExportAuth("missing", path, false); err == nil {
		t.Fatal("导出不存在账号")
	}
	account.AuthError = "授权已撤销"
	if err := s.saveAccount(account); err != nil {
		t.Fatal(err)
	}
	if err := s.ExportAuth(account.ID, path, false); err == nil {
		t.Fatal("导出已失效凭据")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("导出失败写入文件")
	}
}

func TestAuthExportDoesNotChangeKeyringConfiguration(t *testing.T) {
	s := testService(t)
	account := testAccount(t)
	addTestAccount(t, s, account)
	config := []byte("cli_auth_credentials_store = \"keyring\"\n")
	if err := os.WriteFile(filepath.Join(s.cli.Home, "config.toml"), config, 0600); err != nil {
		t.Fatal(err)
	}
	if err := s.ExportAuth(account.ID, filepath.Join(s.cli.Home, "auth.json"), true); err == nil {
		t.Fatal("导出擅自改变 CLI 存储")
	}
	path := filepath.Join(t.TempDir(), "auth.json")
	if err := s.ExportAuth(account.ID, path, false); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(filepath.Join(s.cli.Home, "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(config) {
		t.Fatal("导出改写 CLI 配置")
	}
}
