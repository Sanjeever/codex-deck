package deck

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func waitBackup(t *testing.T, s *Service) {
	t.Helper()
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(5 * time.Millisecond)
	defer tick.Stop()
	for {
		s.mu.Lock()
		done := s.backup.At != nil && !s.backup.Busy && !s.backupPending
		s.mu.Unlock()
		if done {
			return
		}
		select {
		case <-deadline.C:
			t.Fatal("等待自动备份超时")
		case <-tick.C:
		}
	}
}

func TestBackupFirstConnectionAndTwoDeviceConflict(t *testing.T) {
	settings, root, closeServer := testWebDAV(t)
	defer closeServer()
	first := testService(t)
	a := testAccount(t)
	addTestAccount(t, first, a)
	if err := first.SaveSettings(settings); err != nil {
		t.Fatal(err)
	}
	waitBackup(t, first)
	original, err := os.ReadFile(filepath.Join(root, "backup", "codex-deck.json"))
	if err != nil {
		t.Fatal(err)
	}
	second := testService(t)
	if err := second.SaveSettings(settings); err != nil {
		t.Fatal(err)
	}
	waitBackup(t, second)
	if !second.GetState().Backup.Conflict {
		t.Fatal("首次配置覆盖已有备份")
	}
	remote, _ := os.ReadFile(filepath.Join(root, "backup", "codex-deck.json"))
	if string(remote) != string(original) {
		t.Fatal("已有备份被覆盖")
	}
	preview, err := second.PrepareRestore()
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Added) != 1 || preview.ModifiedAt == nil {
		t.Fatal("恢复预览缺少真实更新时间或新增账号")
	}
	if err := second.Restore(preview.ID); err != nil {
		t.Fatal(err)
	}
	if err := first.Rename(a.ID, "设备一"); err != nil {
		t.Fatal(err)
	}
	waitBackup(t, first)
	restoredID := second.GetState().Accounts[0].ID
	if err := second.Rename(restoredID, "设备二"); err != nil {
		t.Fatal(err)
	}
	waitBackup(t, second)
	if !second.GetState().Backup.Conflict {
		t.Fatal("其他设备修改未触发冲突")
	}
	remoteData, err := downloadBackup(settings)
	if err != nil || remoteData.Accounts[0].Alias != "设备一" {
		t.Fatal("冲突写入覆盖其他设备")
	}
	if err := second.OverwriteBackup(); err != nil {
		t.Fatal(err)
	}
	remoteData, err = downloadBackup(settings)
	if err != nil || remoteData.Accounts[0].Alias != "设备二" {
		t.Fatal("显式覆盖未上传")
	}
	// 确认界面打开后另一设备再次写入，条件写仍须阻止覆盖。
	if err := first.Backup(); !errors.Is(err, ErrBackupConflict) {
		t.Fatal("设备一未发现覆盖冲突")
	}
	if err := second.Rename(restoredID, "设备二再次更新"); err != nil {
		t.Fatal(err)
	}
	waitBackup(t, second)
	if err := first.OverwriteBackup(); !errors.Is(err, ErrBackupConflict) {
		t.Fatal("过期覆盖确认仍生效")
	}
}

func TestBackupQueueCoalescesAndDoesNotBlockEditing(t *testing.T) {
	s := testService(t)
	a := testAccount(t)
	addTestAccount(t, s, a)
	entered := make(chan struct{})
	release := make(chan struct{})
	var writes atomic.Int32
	var body []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "HEAD" {
			w.WriteHeader(404)
			return
		}
		if writes.Add(1) == 1 {
			close(entered)
			<-release
		}
		var err error
		body, err = io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		w.Header().Set("ETag", fmt.Sprintf(`"%d"`, writes.Load()))
		w.WriteHeader(201)
	}))
	defer server.Close()
	if err := s.SaveSettings(Settings{WebDAVURL: server.URL}); err != nil {
		t.Fatal(err)
	}
	<-entered
	for _, alias := range []string{"一", "二", "最终"} {
		done := make(chan error, 1)
		go func(value string) { done <- s.Rename(a.ID, value) }(alias)
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(time.Second):
			t.Fatal("后台上传阻塞编辑")
		}
	}
	close(release)
	waitBackup(t, s)
	if writes.Load() != 2 {
		t.Fatalf("待上传旧快照未合并: %d", writes.Load())
	}
	d, err := parseData(body)
	if err != nil || d.Accounts[0].Alias != "最终" {
		t.Fatal("队列未上传最新快照")
	}
}

func TestConditionalWriteDoesNotAcceptMissingETag(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	defer server.Close()
	_, err := writeBackup(context.Background(), Data{Version: Version, Settings: Settings{WebDAVURL: server.URL}, Accounts: []Account{}}, "W/\"weak\"", false)
	if err == nil {
		t.Fatal("弱 ETag 被用于条件写")
	}
}

func TestJianguoyunMetadataAndUnquotedETag(t *testing.T) {
	body, _ := json.Marshal(Data{Version: Version, Accounts: []Account{}})
	var puts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "HEAD" {
			w.Header().Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
			return
		}
		if r.Method == "PUT" {
			puts++
			if r.Header.Get("If-Match") != "actual-tag" {
				t.Error("未使用坚果云原始 ETag")
			}
			body, _ = io.ReadAll(r.Body)
			w.WriteHeader(204)
			return
		}
		w.Header().Set("ETag", "actual-tag")
		w.Write(body)
	}))
	defer server.Close()
	target, _ := url.Parse(server.URL)
	original := http.DefaultTransport
	http.DefaultTransport = davTestTransport{target: target, base: original}
	defer func() { http.DefaultTransport = original }()
	settings := Settings{WebDAVURL: "https://dav.jianguoyun.com/dav/"}
	meta, exists, err := backupMetadata(context.Background(), settings)
	if err != nil || !exists || meta.ETag != "actual-tag" || meta.ModifiedAt == nil {
		t.Fatal("坚果云元数据读取错误")
	}
	tag, err := writeBackup(context.Background(), Data{Version: Version, Settings: settings, Accounts: []Account{}}, meta.ETag, false)
	if err != nil || tag != "actual-tag" || puts != 1 {
		t.Fatalf("坚果云条件上传失败: %v", err)
	}
	_, meta, err = readBackup(context.Background(), settings)
	if err != nil || meta.ModifiedAt == nil {
		t.Fatal("恢复预览丢失 HEAD 更新时间")
	}
}

func TestMissingETagCannotBeConfirmedForOverwrite(t *testing.T) {
	s := testService(t)
	var writes atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "PUT" {
			writes.Add(1)
		}
		w.WriteHeader(200)
	}))
	defer server.Close()
	if err := s.SaveSettings(Settings{WebDAVURL: server.URL}); err != nil {
		t.Fatal(err)
	}
	waitBackup(t, s)
	if !s.GetState().Backup.Conflict {
		t.Fatal("已有无版本备份未要求确认")
	}
	if err := s.OverwriteBackup(); err == nil {
		t.Fatal("无 ETag 仍允许盲覆盖")
	}
	if writes.Load() != 0 {
		t.Fatal("无 ETag 的远端数据被写入")
	}
}
