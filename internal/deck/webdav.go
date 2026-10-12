package deck

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/studio-b12/gowebdav"
)

func webDAVClient(settings Settings) (*gowebdav.Client, error) {
	if err := validateSettings(settings); err != nil {
		return nil, err
	}
	if settings.WebDAVURL == "" {
		return nil, fmt.Errorf("尚未配置 WebDAV")
	}
	client := gowebdav.NewClient(settings.WebDAVURL, settings.WebDAVUsername, settings.WebDAVPassword)
	client.SetTimeout(30 * time.Second)
	return client, nil
}

func uploadBackup(d Data) error {
	client, err := webDAVClient(d.Settings)
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return err
	}
	if err = client.Write("codex-deck.json", b, 0600); err != nil {
		return fmt.Errorf("WebDAV 备份失败: %w", err)
	}
	return nil
}

func downloadBackup(settings Settings) (Data, error) {
	client, err := webDAVClient(settings)
	if err != nil {
		return Data{}, err
	}
	b, err := client.Read("codex-deck.json")
	if err != nil {
		return Data{}, fmt.Errorf("WebDAV 下载失败: %w", err)
	}
	d, err := parseData(b)
	if err != nil {
		return Data{}, fmt.Errorf("远端备份无效: %w", err)
	}
	return d, nil
}
