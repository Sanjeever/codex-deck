package deck

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"codex-deck/internal/network"
)

func TestNetworkSaveReloadAndFailure(t *testing.T) {
	s := testService(t)
	if s.GetState().Network.Mode != "environment" {
		t.Fatal("旧配置默认代理模式错误")
	}
	config := network.Config{Mode: "custom", URL: "http://127.0.0.1:12345", Username: "local-user", Password: "local-password"}
	if err := s.SaveNetwork(config); err != nil {
		t.Fatal(err)
	}
	store, err := OpenStore(s.store.path)
	if err != nil {
		t.Fatal(err)
	}
	if store.Snapshot().Network == nil || *store.Snapshot().Network != config {
		t.Fatal("代理配置未持久保存")
	}
	if err := s.SaveNetwork(network.Config{Mode: "custom", URL: "socks5://localhost:12345"}); err == nil {
		t.Fatal("接受无效代理")
	}
	if s.GetState().Network != config {
		t.Fatal("无效配置覆盖已有设置")
	}
	oldPath := s.store.path
	s.store.path = filepath.Join(t.TempDir(), "directory")
	if err := os.Mkdir(s.store.path, 0700); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveNetwork(network.Config{Mode: "direct"}); err == nil {
		t.Fatal("保存失败未报错")
	}
	s.store.path = oldPath
	if s.GetState().Network != config {
		t.Fatal("保存失败改变内存设置")
	}
}

func TestAllServiceRequestsShareProxy(t *testing.T) {
	s := testService(t)
	var paths []string
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if r.URL.Path == "/quota" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{}`))
			return
		}
		w.Header().Set("ETag", `"proxy-etag"`)
		if r.Method == "MKCOL" {
			w.WriteHeader(http.StatusCreated)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer proxy.Close()
	if err := s.SaveNetwork(network.Config{Mode: "custom", URL: proxy.URL}); err != nil {
		t.Fatal(err)
	}
	response, err := s.oauth.Client.Get("http://service.invalid/oauth")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	var payload map[string]any
	if _, err := requestJSON(s.ctx, s.backend.Client, http.MethodGet, "http://service.invalid/quota", "", nil, nil, &payload); err != nil {
		t.Fatal(err)
	}
	settings := Settings{WebDAVURL: "http://service.invalid/backup"}
	if _, _, err := backupMetadata(s.ctx, settings); err != nil {
		t.Fatal(err)
	}
	client, _, err := webDAVClient(settings)
	if err != nil {
		t.Fatal(err)
	}
	client.SetTransport(backupContextTransport{ctx: s.ctx})
	if err := client.Mkdir("directory", 0700); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"/oauth", "/quota", "/backup/codex-deck.json", "/backup/directory/"} {
		found := false
		for _, path := range paths {
			if path == expected {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("请求 %s 未走代理：%v", expected, paths)
		}
	}
}

func TestNetworkExcludedFromBackupAndRetainedOnRestore(t *testing.T) {
	s := testService(t)
	config := network.Config{Mode: "direct"}
	if err := s.SaveNetwork(config); err != nil {
		t.Fatal(err)
	}
	var saved []byte
	remote := Data{Version: Version, Accounts: []Account{}, Network: &network.Config{Mode: "unsupported-on-this-device", URL: "http://other-device:9999", Password: "remote-secret", Username: "remote-user"}}
	body, err := json.Marshal(remote)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"backup-version"`)
		switch r.Method {
		case "HEAD":
			w.WriteHeader(http.StatusOK)
		case "GET":
			w.Write(body)
		case "PUT":
			saved, _ = io.ReadAll(r.Body)
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer server.Close()
	settings := Settings{WebDAVURL: server.URL}
	local := s.store.Snapshot()
	local.Settings = settings
	if _, err := writeBackup(network.WithRouter(context.Background(), s.HTTP), local, `"backup-version"`, false); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(saved), `"network"`) || strings.Contains(string(saved), "remote-secret") {
		t.Fatal("备份包含设备代理")
	}
	if err := s.store.Update(func(d *Data) error { d.Settings = settings; return nil }); err != nil {
		t.Fatal(err)
	}
	preview, err := s.PrepareRestore()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Restore(preview.ID); err != nil {
		t.Fatal(err)
	}
	if s.GetState().Network != config || *s.store.Snapshot().Network != config {
		t.Fatal("恢复覆盖本机代理")
	}
}
