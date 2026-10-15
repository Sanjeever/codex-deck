package deck

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
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
	var requests sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Lock()
		defer requests.Unlock()
		user, password, ok := r.BasicAuth()
		if !ok || user != "test-user" || password != "test-password" {
			w.Header().Set("WWW-Authenticate", `Basic realm="local-test"`)
			w.WriteHeader(401)
			return
		}
		// x/net/webdav 当前不处理 PUT 条件头，测试服务补齐 RFC 条件语义。
		if r.Method == http.MethodPut {
			head := httptest.NewRecorder()
			request := r.Clone(r.Context())
			request.Method = http.MethodHead
			handler.ServeHTTP(head, request)
			exists := head.Code == 200
			if (r.Header.Get("If-None-Match") == "*" && exists) || (r.Header.Get("If-Match") != "" && (!exists || r.Header.Get("If-Match") != head.Header().Get("ETag"))) {
				w.WriteHeader(412)
				return
			}
		}
		handler.ServeHTTP(w, r)
	}))
	return Settings{WebDAVURL: server.URL + "/backup/", WebDAVUsername: "test-user", WebDAVPassword: "test-password"}, root, server.Close
}

func TestLegacyWebDAVBackupRecovery(t *testing.T) {
	settings, root, closeServer := testWebDAV(t)
	defer closeServer()
	d := Data{Version: Version, Settings: settings, Accounts: []Account{testAccount(t)}}
	if err := uploadBackup(d); err != nil {
		t.Fatal(err)
	}
	files, err := listBackupFiles(context.Background(), settings)
	if err != nil || len(files) != 1 || !files[0].Legacy || files[0].Hostname != "" {
		t.Fatalf("旧版备份来源错误: %v, %v", files, err)
	}
	restored, err := downloadBackup(settings)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Accounts[0].Alias != d.Accounts[0].Alias || restored.Accounts[0].Tokens.RefreshToken != d.Accounts[0].Tokens.RefreshToken {
		t.Fatal("备份内容不完整")
	}
	entries, err := os.ReadDir(filepath.Join(root, "backup"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "codex-deck.json" {
		t.Fatal("读取旧版备份修改远端文件")
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
	if _, err := writeSnapshot(context.Background(), Data{Version: Version, Settings: settings, Accounts: []Account{}}); err == nil {
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

type davTestTransport struct {
	target *url.URL
	base   http.RoundTripper
}

func (transport davTestTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	forward := request.Clone(request.Context())
	forward.URL.Scheme = transport.target.Scheme
	forward.URL.Host = transport.target.Host
	base := transport.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(forward)
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

// 测试夹具直接写入远端，用于模拟另一设备与无效备份。
func uploadBackup(d Data) error {
	client, path, err := webDAVClient(d.Settings)
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return err
	}
	if err = client.Write(path, b, 0600); err != nil {
		return fmt.Errorf("WebDAV 备份失败: %w", err)
	}
	return nil
}

func downloadBackup(settings Settings) (Data, error) {
	files, err := listBackupFiles(context.Background(), settings)
	if err != nil {
		return Data{}, err
	}
	if len(files) == 0 {
		return Data{}, fmt.Errorf("没有备份")
	}
	file := files[0]
	for _, candidate := range files {
		if candidate.Legacy {
			file = candidate
		}
	}
	d, _, _, err := readSnapshot(context.Background(), settings, file.ID)
	return d, err
}

func prepareRestoreTest(t *testing.T, s *Service) (RestorePreview, error) {
	t.Helper()
	files, err := s.ListBackups()
	if err != nil {
		return RestorePreview{}, err
	}
	if len(files) == 0 {
		return RestorePreview{}, fmt.Errorf("没有备份")
	}
	file := files[0]
	for _, candidate := range files {
		if candidate.Legacy {
			file = candidate
		}
	}
	return s.PrepareRestore(file.ID)
}

func latestBackupBytes(settings Settings) ([]byte, error) {
	files, err := listBackupFiles(context.Background(), settings)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("没有备份")
	}
	_, _, body, err := readSnapshot(context.Background(), settings, files[0].ID)
	return body, err
}
