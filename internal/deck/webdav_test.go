package deck

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
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

type davTestTransport struct{ target *url.URL }

func (transport davTestTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	forward := request.Clone(request.Context())
	forward.URL.Scheme = transport.target.Scheme
	forward.URL.Host = transport.target.Host
	return http.DefaultTransport.RoundTrip(forward)
}

func TestJianguoyunRootBackupDirectory(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "dav"), 0700); err != nil {
		t.Fatal(err)
	}
	handler := &webdav.Handler{FileSystem: webdav.Dir(root), LockSystem: webdav.NewMemLS()}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && path.Dir(r.URL.Path) == "/dav" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		handler.ServeHTTP(w, r)
	}))
	defer server.Close()
	target, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	for _, address := range []string{"https://dav.jianguoyun.com/dav/", "https://dav.jianguoyun.com/dav"} {
		client, filename, err := webDAVClient(Settings{WebDAVURL: address})
		if err != nil {
			t.Fatal(err)
		}
		if filename != "codex-deck/codex-deck.json" {
			t.Fatal("坚果云根地址未使用实际目录")
		}
		client.SetTransport(davTestTransport{target: target})
		body := []byte(`{"version":1,"accounts":[],"settings":{}}`)
		if err := client.Write(filename, body, 0600); err != nil {
			t.Fatal(err)
		}
		got, err := client.Read(filename)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != string(body) {
			t.Fatal("备份与恢复路径不一致")
		}
	}
	for _, address := range []string{"https://dav.jianguoyun.com/dav/custom/", "https://example.test/dav/"} {
		_, filename, err := webDAVClient(Settings{WebDAVURL: address})
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(filename, "/") {
			t.Fatal("改动其他 WebDAV 目录的备份位置")
		}
	}
}
