package deck

import (
	"context"
	"os"
	"testing"
)

// 显式提供测试服务凭据时执行；使用独立空目录，只清理本次创建的快照。
func TestWebDAVLiveBackup(t *testing.T) {
	address := os.Getenv("CODEX_DECK_TEST_WEBDAV_URL")
	if address == "" {
		t.Skip("未配置 WebDAV 集成测试服务")
	}
	settings := Settings{WebDAVURL: address, WebDAVUsername: os.Getenv("CODEX_DECK_TEST_WEBDAV_USER"), WebDAVPassword: os.Getenv("CODEX_DECK_TEST_WEBDAV_PASSWORD")}
	client, _, err := webDAVClient(settings)
	if err != nil {
		t.Fatal(err)
	}
	files, err := listBackupFiles(context.Background(), settings)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 0 {
		t.Skip("远端已有备份，请提供独立空目录")
	}
	d := Data{Version: Version, Settings: settings, Accounts: []Account{}}
	file, err := writeSnapshot(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := client.Remove(file.ID); err != nil {
			t.Error("清理测试备份失败", err)
		}
	})
	got, _, _, err := readSnapshot(context.Background(), settings, file.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != Version || len(got.Accounts) != 0 || got.Settings != settings {
		t.Fatal("真实 WebDAV 备份读取内容不一致")
	}
}
