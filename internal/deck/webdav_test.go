package deck

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/net/webdav"
)

func testWebDAV(t *testing.T) (Settings, string, func()) {
	t.Helper()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "backup"), 0700); err != nil {
		t.Fatal(err)
	}
	handler := &webdav.Handler{FileSystem: webdav.Dir(root), LockSystem: webdav.NewMemLS()}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, password, ok := r.BasicAuth()
		if !ok || user != "test-user" || password != "test-password" {
			w.Header().Set("WWW-Authenticate", `Basic realm="local-test"`)
			w.WriteHeader(401)
			return
		}
		handler.ServeHTTP(w, r)
	}))
	return Settings{WebDAVURL: server.URL + "/backup/", WebDAVUsername: "test-user", WebDAVPassword: "test-password"}, root, server.Close
}

func TestWebDAVUploadReplaceAndDownload(t *testing.T) {
	settings, root, closeServer := testWebDAV(t)
	defer closeServer()
	d := Data{Version: Version, Settings: settings, Accounts: []Account{testAccount(t)}}
	if err := uploadBackup(d); err != nil {
		t.Fatal(err)
	}
	d.Accounts[0].Alias = "新别名"
	if err := uploadBackup(d); err != nil {
		t.Fatal(err)
	}
	restored, err := downloadBackup(settings)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Accounts[0].Alias != "新别名" || restored.Accounts[0].Tokens.RefreshToken != d.Accounts[0].Tokens.RefreshToken {
		t.Fatal("备份内容不完整")
	}
	entries, err := os.ReadDir(filepath.Join(root, "backup"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "codex-deck.json" {
		t.Fatal("产生历史版本或额外文件")
	}
}

func TestWebDAVFailureAndInvalidData(t *testing.T) {
	settings, root, closeServer := testWebDAV(t)
	defer closeServer()
	if _, err := downloadBackup(settings); err == nil {
		t.Fatal("不存在的备份应报错")
	}
	if err := os.WriteFile(filepath.Join(root, "backup", "codex-deck.json"), []byte(`{"version":1,"accounts":[{}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := downloadBackup(settings); err == nil {
		t.Fatal("接受无效账号数据")
	}
	settings.WebDAVPassword = "wrong"
	if err := uploadBackup(Data{Version: Version, Settings: settings, Accounts: []Account{}}); err == nil {
		t.Fatal("未暴露认证失败")
	}
	var malformed Data
	if err := json.Unmarshal([]byte(`{"version":99}`), &malformed); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "backup", "codex-deck.json"), []byte(`{"version":99}`), 0600); err != nil {
		t.Fatal(err)
	}
	settings.WebDAVPassword = "test-password"
	if _, err := downloadBackup(settings); err == nil {
		t.Fatal("接受未知版本")
	}
}
