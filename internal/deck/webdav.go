package deck

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"codex-deck/internal/network"

	"github.com/studio-b12/gowebdav"
)

// 同时返回旧版文件位置，用于列出并恢复升级前的备份。
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
	filename := "codex-deck.json"
	// 坚果云旧版根地址备份使用实际同步目录。
	if strings.EqualFold(address.Hostname(), "dav.jianguoyun.com") && strings.TrimSuffix(address.Path, "/") == "/dav" {
		filename = "codex-deck/codex-deck.json"
	}
	client := gowebdav.NewClient(settings.WebDAVURL, settings.WebDAVUsername, settings.WebDAVPassword)
	client.SetTimeout(30 * time.Second)
	return client, filename, nil
}

func backupFileRequest(ctx context.Context, settings Settings, filename, method string, body []byte, headers map[string]string) (*http.Response, error) {
	address, err := url.JoinPath(settings.WebDAVURL, filename)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, method, address, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.SetBasicAuth(settings.WebDAVUsername, settings.WebDAVPassword)
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	return (&http.Client{Transport: network.FromContext(ctx), Timeout: 30 * time.Second}).Do(request)
}

type backupContextTransport struct{ ctx context.Context }

func parseBackupData(body []byte) (Data, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		return Data{}, err
	}
	delete(fields, "network")
	body, err := json.Marshal(fields)
	if err != nil {
		return Data{}, err
	}
	return parseData(body)
}

func (t backupContextTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return network.FromContext(t.ctx).RoundTrip(r.WithContext(t.ctx))
}
