package deck

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"codex-deck/internal/network"
)

func TestSnapshotHistoryAndLegacy(t *testing.T) {
	settings, root, closeServer := testWebDAV(t)
	defer closeServer()
	ctx := context.Background()
	files, err := listBackupFiles(ctx, settings)
	if err != nil || len(files) != 0 {
		t.Fatalf("空目录列表错误: %v, %v", files, err)
	}
	d := Data{Version: Version, Settings: settings, Accounts: []Account{testAccount(t)}, Network: &network.Config{Mode: "direct", Password: "device-secret"}}
	if err := uploadBackup(d); err != nil {
		t.Fatal(err)
	}
	legacy, err := os.ReadFile(filepath.Join(root, "backup", "codex-deck.json"))
	if err != nil {
		t.Fatal(err)
	}
	first, err := writeSnapshot(ctx, d)
	if err != nil {
		t.Fatal(err)
	}
	d.Accounts[0].Alias = "另一个快照"
	second, err := writeSnapshot(ctx, d)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == second.ID || first.Hostname == "" {
		t.Fatal("快照文件名重复或缺少来源设备")
	}
	files, err = listBackupFiles(ctx, settings)
	if err != nil || len(files) != 3 {
		t.Fatalf("历史列表错误: %v, %v", files, err)
	}
	if files[0].ID != second.ID || files[1].ID != first.ID || !files[2].Legacy {
		t.Fatalf("快照排序或旧版来源错误: %v", files)
	}
	restored, metadata, body, err := readSnapshot(ctx, settings, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Accounts[0].Alias == d.Accounts[0].Alias || restored.Accounts[0].Tokens.RefreshToken != d.Accounts[0].Tokens.RefreshToken {
		t.Fatal("历史快照被覆盖或丢失凭据")
	}
	if restored.Network != nil || strings.Contains(string(body), "device-secret") || metadata.Hostname != first.Hostname || metadata.CreatedAt.IsZero() {
		t.Fatal("备份来源或本机配置边界错误")
	}
	after, err := os.ReadFile(filepath.Join(root, "backup", "codex-deck.json"))
	if err != nil || string(after) != string(legacy) {
		t.Fatal("上传快照改变旧版文件")
	}
}

func TestSnapshotNamesAndDirectoryFiltering(t *testing.T) {
	at := time.Date(2026, 10, 15, 16, 30, 12, 123000000, time.FixedZone("local", 8*3600))
	name := snapshotName(at, "Mac_Book/测试:设备")
	file, ok := snapshotFile(name)
	if !ok || file.Hostname != "Mac_Book-测试-设备" || !file.CreatedAt.Equal(at) || strings.ContainsAny(name, "/\\:") {
		t.Fatalf("跨平台名称错误: %s, %v", name, file)
	}
	settings, root, closeServer := testWebDAV(t)
	defer closeServer()
	dir := filepath.Join(root, "backup", "codex-deck")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"unrelated.json", "codex-deck_invalid_host_12345678.json", "codex-deck.json.bak"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("{}"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, snapshotName(at, "directory")), 0700); err != nil {
		t.Fatal(err)
	}
	files, err := listBackupFiles(context.Background(), settings)
	if err != nil || len(files) != 0 {
		t.Fatalf("备份列表包含无关文件或目录: %v, %v", files, err)
	}
}

func TestSnapshotUploadWithoutETagAndReadFailures(t *testing.T) {
	var putPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			putPath = r.URL.Path
			if r.Header.Get("If-None-Match") != "*" {
				t.Error("快照创建缺少条件头")
			}
			w.WriteHeader(http.StatusCreated)
			return
		}
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()
	settings := Settings{WebDAVURL: server.URL}
	file, err := writeSnapshot(context.Background(), Data{Version: Version, Settings: settings, Accounts: []Account{}})
	if err != nil || putPath != "/"+file.ID {
		t.Fatalf("无 ETag 上传失败: %v", err)
	}
	if _, err := listBackupFiles(context.Background(), settings); err == nil {
		t.Fatal("列表失败被隐藏")
	}
	if _, _, _, err := readSnapshot(context.Background(), settings, file.ID); err == nil {
		t.Fatal("下载失败被隐藏")
	}
	invalid := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"version": 99})
	}))
	defer invalid.Close()
	if _, _, _, err := readSnapshot(context.Background(), Settings{WebDAVURL: invalid.URL}, file.ID); err == nil {
		t.Fatal("接受无效备份")
	}
}

func TestSnapshotReadUsesDownloadedFileMetadata(t *testing.T) {
	at := time.Date(2026, 10, 15, 8, 30, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Last-Modified", at.Format(http.TimeFormat))
		w.Write([]byte(`{"version":1,"accounts":[],"backup_metadata":{"hostname":"原始设备/名称","created_at":"2026-10-15T08:00:00Z"}}`))
	}))
	defer server.Close()
	_, metadata, _, err := readSnapshot(context.Background(), Settings{WebDAVURL: server.URL}, "codex-deck/file.json")
	if err != nil || metadata.Hostname != "原始设备/名称" || metadata.CreatedAt.IsZero() || metadata.ModifiedAt == nil || !metadata.ModifiedAt.Equal(at) {
		t.Fatalf("下载未保留真实来源或更新时间: %v, %v", metadata, err)
	}
}
