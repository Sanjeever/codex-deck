package deck

import (
	"net/http"
	"os"
	"testing"

	"github.com/studio-b12/gowebdav"
)

// 显式提供测试服务凭据时执行；远端已有备份则跳过，避免覆盖真实数据。
func TestWebDAVLiveBackup(t *testing.T) {
	address := os.Getenv("CODEX_DECK_TEST_WEBDAV_URL")
	if address == "" {
		t.Skip("未配置 WebDAV 集成测试服务")
	}
	settings := Settings{WebDAVURL: address, WebDAVUsername: os.Getenv("CODEX_DECK_TEST_WEBDAV_USER"), WebDAVPassword: os.Getenv("CODEX_DECK_TEST_WEBDAV_PASSWORD")}
	client, filename, err := webDAVClient(settings)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Stat(filename); err == nil {
		t.Skip("远端已有备份，不覆盖现有数据")
	} else if !gowebdav.IsErrNotFound(err) && !gowebdav.IsErrCode(err, http.StatusConflict) {
		t.Fatal(err)
	}
	d := Data{Version: Version, Settings: settings, Accounts: []Account{}}
	if err := uploadBackup(d); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := client.Remove(filename); err != nil {
			t.Error("清理测试备份失败", err)
		}
	})
	got, err := downloadBackup(settings)
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != Version || len(got.Accounts) != 0 || got.Settings != settings {
		t.Fatal("真实 WebDAV 备份读取内容不一致")
	}
}
