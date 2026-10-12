package deck

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCLIFileStorePreservesConfig(t *testing.T) {
	c := CLI{Home: t.TempDir()}
	original := "# personal settings\ncli_auth_credentials_store = 'keyring' # keep comment\nmodel = \"test-model\"\n[profiles.test]\ncli_auth_credentials_store = \"auto\"\n"
	path := filepath.Join(c.Home, "config.toml")
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	a := testAccount(t)
	if err := c.Write(a.Tokens); err == nil {
		t.Fatal("未确认便写入")
	}
	if err := c.EnableFileStore(); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	want := "# personal settings\ncli_auth_credentials_store = \"file\" # keep comment\nmodel = \"test-model\"\n[profiles.test]\ncli_auth_credentials_store = \"auto\"\n"
	if string(b) != want {
		t.Fatalf("改动其他配置: %s", b)
	}
	if err := c.Write(a.Tokens); err != nil {
		t.Fatal(err)
	}
	auth, i, err := c.Read()
	if err != nil {
		t.Fatal(err)
	}
	if auth.AuthMode != "chatgpt" || auth.APIKey != nil || auth.Tokens.AccountID != a.AccountID || !identityMatches(a, i) {
		t.Fatal("CLI 格式或身份错误")
	}
	b, _ = os.ReadFile(filepath.Join(c.Home, "auth.json"))
	if string(b) == "" {
		t.Fatal("未写入认证")
	}
}

func TestCLIRefreshDetectsConcurrentChange(t *testing.T) {
	c := CLI{Home: t.TempDir()}
	a := testAccount(t)
	if err := c.Write(a.Tokens); err != nil {
		t.Fatal(err)
	}
	a.Tokens.RefreshToken = "cli-new-refresh"
	if err := c.Write(a.Tokens); err != nil {
		t.Fatal(err)
	}
	old := testAccount(t)
	updated, current, err := c.ImportCurrent(old)
	if err != nil {
		t.Fatal(err)
	}
	if !current || updated.Tokens.RefreshToken != "cli-new-refresh" {
		t.Fatal("未采用 CLI 最新令牌")
	}
	next := old.Tokens
	next.RefreshToken = "gui-new-refresh"
	if err := c.WriteIfUnchanged(old, next); err == nil {
		t.Fatal("覆盖 CLI 并发更新")
	}
	auth, _, err := c.Read()
	if err != nil {
		t.Fatal(err)
	}
	if auth.Tokens.RefreshToken != "cli-new-refresh" {
		t.Fatal("CLI 认证被覆盖")
	}
	if err := c.WriteIfUnchanged(updated, next); err != nil {
		t.Fatal(err)
	}
}
