package deck

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
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

func TestRestorePreviewCurrentAccountAndReimport(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"plan_type":"plus"}`))
	}))
	defer server.Close()
	for _, name := range []string{"same_identity", "different_user", "different_workspace", "cli_changed", "logged_out"} {
		t.Run(name, func(t *testing.T) {
			s := testService(t)
			s.backend = Backend{Client: server.Client(), BaseURL: server.URL}
			current := testAccount(t)
			addTestAccount(t, s, current)
			if err := s.Switch(current.ID, false); err != nil {
				t.Fatal(err)
			}
			remote := current
			remote.ID = "remote-record"
			switch name {
			case "different_user":
				remote.UserID = "other-user"
			case "different_workspace":
				remote.AccountID = "other-workspace"
			case "cli_changed":
				other := current
				other.ID = "other-record"
				other.UserID = "other-user"
				other.Tokens.IDToken = testToken(t, other.UserID, other.AccountID, time.Now().Add(time.Hour))
				other.Tokens.AccessToken = other.Tokens.IDToken
				addTestAccount(t, s, other)
				if err := s.cli.Write(other.Tokens); err != nil {
					t.Fatal(err)
				}
			case "logged_out":
				if err := os.Remove(filepath.Join(s.cli.Home, "auth.json")); err != nil {
					t.Fatal(err)
				}
			}
			remote.Tokens.AccountID = remote.AccountID
			remote.Tokens.IDToken = testToken(t, remote.UserID, remote.AccountID, time.Now().Add(time.Hour))
			remote.Tokens.AccessToken = remote.Tokens.IDToken
			settings, _, closeServer := testWebDAV(t)
			defer closeServer()
			if err := s.SaveSettings(settings); err != nil {
				t.Fatal(err)
			}
			waitBackup(t, s)
			if err := uploadBackup(Data{Version: Version, Settings: settings, Accounts: []Account{remote}}); err != nil {
				t.Fatal(err)
			}
			preview, err := prepareRestoreTest(t, s)
			if err != nil {
				t.Fatal(err)
			}
			wantRemoval := name != "same_identity" && name != "logged_out"
			if preview.RemovesCurrent != wantRemoval {
				t.Fatal("恢复预览未按 CLI 实际身份判断当前账号是否移除")
			}
			if err := s.Restore(preview.ID); err != nil {
				t.Fatal(err)
			}
			// 认证文件变更事件会在自动查询前同步 CLI 实际身份。
			s.mu.Lock()
			s.syncCLIAccount(true)
			s.mu.Unlock()
			if state := s.RefreshAutomatic(); len(state.Accounts) != 1 {
				t.Fatal("自动刷新重新加入恢复移除的当前账号")
			}
			wantCount := 1
			if wantRemoval {
				wantCount = 2
			}
			if state := s.RefreshAll(); len(state.Accounts) != wantCount {
				t.Fatal("显式刷新未按实际 CLI 身份重新导入账号")
			}
		})
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

func TestBackupTwoDevicesKeepIndependentSnapshots(t *testing.T) {
	settings, _, closeServer := testWebDAV(t)
	defer closeServer()
	first, second := testService(t), testService(t)
	a := testAccount(t)
	addTestAccount(t, first, a)
	for _, s := range []*Service{first, second} {
		if err := s.SaveSettings(settings); err != nil {
			t.Fatal(err)
		}
		waitBackup(t, s)
		if s.GetState().Backup.Error != "" {
			t.Fatal(s.GetState().Backup.Error)
		}
	}
	files, err := second.ListBackups()
	if err != nil || len(files) != 2 {
		t.Fatalf("设备未分别保存快照: %v, %v", files, err)
	}
	var firstID string
	for _, file := range files {
		d, _, _, err := readSnapshot(first.ctx, settings, file.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(d.Accounts) == 1 {
			firstID = file.ID
		}
	}
	preview, err := second.PrepareRestore(firstID)
	if err != nil || len(preview.Added) != 1 || preview.File.Hostname == "" || preview.File.CreatedAt == nil {
		t.Fatalf("选中文件预览错误: %v, %v", preview, err)
	}
	if err := first.Backup(); err != nil {
		t.Fatal(err)
	}
	if err := second.Restore(preview.ID); err != nil {
		t.Fatal(err)
	}
	files, err = first.ListBackups()
	if err != nil || len(files) != 3 {
		t.Fatal("恢复自动上传")
	}
	for i, s := range []*Service{first, second} {
		if err := s.Rename(a.ID, fmt.Sprintf("设备%d", i+1)); err != nil {
			t.Fatal(err)
		}
		waitBackup(t, s)
	}
	files, err = first.ListBackups()
	if err != nil || len(files) != 5 {
		t.Fatalf("设备修改覆盖旧快照: %v, %v", files, err)
	}
	d, _, _, err := readSnapshot(first.ctx, settings, firstID)
	if err != nil || d.Accounts[0].Alias != a.Alias {
		t.Fatal("原快照被修改")
	}
}

func TestRestoreRejectsUnlistedChangedAndDeletedFiles(t *testing.T) {
	settings, root, closeServer := testWebDAV(t)
	defer closeServer()
	s := testService(t)
	if err := s.SaveSettings(settings); err != nil {
		t.Fatal(err)
	}
	waitBackup(t, s)
	files, err := s.ListBackups()
	if err != nil || len(files) != 1 {
		t.Fatal(err)
	}
	for _, filename := range []string{"../private.json", "https://other-host/file", "unlisted.json"} {
		if _, err := s.PrepareRestore(filename); err == nil {
			t.Fatalf("允许恢复未列出的文件: %s", filename)
		}
	}
	preview, err := s.PrepareRestore(files[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	before := s.store.Snapshot()
	remotePath := filepath.Join(root, "backup", filepath.FromSlash(files[0].ID))
	if err := os.WriteFile(remotePath, []byte(`{"version":1,"accounts":[],"settings":{},"preferences":{"privacy":true}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := s.Restore(preview.ID); err == nil || s.store.Snapshot().Preferences != before.Preferences {
		t.Fatal("文件改变仍恢复或改写本地数据")
	}
	preview, err = s.PrepareRestore(files[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(remotePath); err != nil {
		t.Fatal(err)
	}
	if err := s.Restore(preview.ID); err == nil || s.store.Snapshot().Settings != before.Settings {
		t.Fatal("文件删除仍恢复或改写本地数据")
	}
}
