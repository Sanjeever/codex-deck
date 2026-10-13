package deck

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"codex-deck/internal/network"
)

func TestConfigurationSavePersistsTogetherAndPreservesOtherState(t *testing.T) {
	s := testService(t)
	a := testAccount(t)
	addTestAccount(t, s, a)
	if err := s.SavePreferences(Preferences{Privacy: true, Sort: "quota"}); err != nil {
		t.Fatal(err)
	}
	before := s.store.Snapshot()
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	defer proxy.Close()
	config := Configuration{
		Network:     network.Config{Mode: "custom", URL: " " + proxy.URL + " "},
		Settings:    Settings{WebDAVUsername: "backup-user", WebDAVPassword: "backup-password"},
		Preferences: ConfigurationPreferences{LowQuotaNotify: true, LowQuotaThreshold: 20, DisableUpdateCheck: true},
	}
	saved, err := s.SaveConfiguration(config)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Network.URL != proxy.URL {
		t.Fatal("未返回规范化配置", saved.Network)
	}
	store, err := OpenStore(s.store.path)
	if err != nil {
		t.Fatal(err)
	}
	d := store.Snapshot()
	if *d.Network != saved.Network || d.Settings != config.Settings || !d.Preferences.LowQuotaNotify || d.Preferences.LowQuotaThreshold != 20 || !d.Preferences.DisableUpdateCheck {
		t.Fatal("设置未一起保存")
	}
	if !d.Preferences.Privacy || d.Preferences.Sort != "quota" || !reflect.DeepEqual(d.Accounts, before.Accounts) {
		t.Fatal("覆盖其他用户状态")
	}
	response, err := s.oauth.Client.Get("http://service.invalid/configuration-test")
	if err != nil {
		t.Fatal("保存后未采用新代理", err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		t.Fatal("请求未到达代理")
	}
}

func TestConfigurationFailureDoesNotPartiallySaveOrChangeRoute(t *testing.T) {
	s := testService(t)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	defer proxy.Close()
	if err := s.SaveNetwork(network.Config{Mode: "custom", URL: proxy.URL}); err != nil {
		t.Fatal(err)
	}
	before := s.store.Snapshot()
	config := Configuration{Network: network.Config{Mode: "direct"}, Preferences: ConfigurationPreferences{LowQuotaNotify: true, LowQuotaThreshold: 20}}
	for _, tc := range []struct {
		name, section string
		change        func(*Configuration)
	}{
		{"network", "network", func(c *Configuration) { c.Network = network.Config{Mode: "custom", URL: "socks5://localhost:12345"} }},
		{"backup", "backup", func(c *Configuration) { c.Settings.WebDAVURL = "not-a-url" }},
		{"notification", "general", func(c *Configuration) { c.Preferences.LowQuotaThreshold = 100 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			invalid := config
			tc.change(&invalid)
			_, err := s.SaveConfiguration(invalid)
			var fieldError *ConfigurationError
			if !errors.As(err, &fieldError) || fieldError.Section != tc.section {
				t.Fatal("未标识出错分类", err)
			}
			if !reflect.DeepEqual(s.store.Snapshot(), before) {
				t.Fatal("校验失败仍保存部分设置")
			}
		})
	}
	oldPath := s.store.path
	s.store.path = filepath.Join(t.TempDir(), "directory")
	if err := os.Mkdir(s.store.path, 0700); err != nil {
		t.Fatal(err)
	}
	_, err := s.SaveConfiguration(config)
	s.store.path = oldPath
	if err == nil || !reflect.DeepEqual(s.store.Snapshot(), before) {
		t.Fatal("写入失败仍改变设置", err)
	}
	store, err := OpenStore(oldPath)
	if err != nil || !reflect.DeepEqual(store.Snapshot(), before) {
		t.Fatal("失败改变磁盘配置", err)
	}
	response, err := s.oauth.Client.Get("http://service.invalid/unchanged-route")
	if err != nil {
		t.Fatal("失败改变代理路由", err)
	}
	response.Body.Close()
}

func TestConfigurationProxyOnlyDoesNotWaitForBackupOrQueueUpload(t *testing.T) {
	s := testService(t)
	if err := s.store.Update(func(d *Data) error {
		d.Settings.WebDAVURL = "http://backup.invalid"
		d.Sync = BackupSync{ETag: "keep-tag"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	d := s.store.Snapshot()
	s.backupIO.Lock()
	done := make(chan error, 1)
	go func() {
		_, err := s.SaveConfiguration(Configuration{Network: network.Config{Mode: "direct"}, Settings: d.Settings})
		done <- err
	}()
	select {
	case err := <-done:
		s.backupIO.Unlock()
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		s.backupIO.Unlock()
		<-done
		t.Fatal("仅修改代理却等待正在上传的备份")
	}
	if s.store.Snapshot().Sync != d.Sync || s.backupPending {
		t.Fatal("仅修改代理改变备份版本或触发上传")
	}
}
