package deck

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/studio-b12/gowebdav"
)

func webDAVClient(settings Settings) (*gowebdav.Client, string, error) {
	if err := validateSettings(settings); err != nil {
		return nil, "", err
	}
	if settings.WebDAVURL == "" {
		return nil, "", fmt.Errorf("尚未配置 WebDAV")
	}
	address, err := url.Parse(settings.WebDAVURL)
	if err != nil {
		return nil, "", err
	}
	path := "codex-deck.json"
	// 坚果云 /dav/ 是同步目录列表，文件必须放入实际同步目录。
	if strings.EqualFold(address.Hostname(), "dav.jianguoyun.com") && strings.TrimSuffix(address.Path, "/") == "/dav" {
		path = "codex-deck/codex-deck.json"
	}
	client := gowebdav.NewClient(settings.WebDAVURL, settings.WebDAVUsername, settings.WebDAVPassword)
	client.SetTimeout(30 * time.Second)
	return client, path, nil
}

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
	client, path, err := webDAVClient(settings)
	if err != nil {
		return Data{}, err
	}
	b, err := client.Read(path)
	if err != nil {
		return Data{}, fmt.Errorf("WebDAV 下载失败: %w", err)
	}
	d, err := parseData(b)
	if err != nil {
		return Data{}, fmt.Errorf("远端备份无效: %w", err)
	}
	return d, nil
}
