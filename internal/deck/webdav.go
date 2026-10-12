package deck

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
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

var ErrBackupConflict = errors.New("远端备份已存在或被其他设备修改，请选择恢复或确认覆盖")

type remoteBackup struct {
	ETag       string
	ModifiedAt *time.Time
}

func backupRequest(ctx context.Context, settings Settings, method string, body []byte, headers map[string]string) (*http.Response, error) {
	_, filename, err := webDAVClient(settings)
	if err != nil {
		return nil, err
	}
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
	return (&http.Client{Timeout: 30 * time.Second}).Do(request)
}

func remoteMetadata(response *http.Response) remoteBackup {
	meta := remoteBackup{ETag: response.Header.Get("ETag")}
	if at, err := http.ParseTime(response.Header.Get("Last-Modified")); err == nil {
		meta.ModifiedAt = &at
	}
	return meta
}

func backupMetadata(ctx context.Context, settings Settings) (remoteBackup, bool, error) {
	response, err := backupRequest(ctx, settings, http.MethodHead, nil, nil)
	if err != nil {
		return remoteBackup{}, false, err
	}
	defer response.Body.Close()
	if response.StatusCode == 404 {
		return remoteBackup{}, false, nil
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return remoteBackup{}, false, fmt.Errorf("检查 WebDAV 备份失败: HTTP %d", response.StatusCode)
	}
	meta := remoteMetadata(response)
	if meta.ETag == "" {
		// 坚果云 HEAD 没有 ETag，从同一资源的 GET 获取实际版本。
		read, err := backupRequest(ctx, settings, http.MethodGet, nil, nil)
		if err != nil {
			return remoteBackup{}, false, err
		}
		defer read.Body.Close()
		if read.StatusCode != 200 {
			return remoteBackup{}, false, fmt.Errorf("读取 WebDAV 版本失败: HTTP %d", read.StatusCode)
		}
		readMeta := remoteMetadata(read)
		if readMeta.ModifiedAt == nil {
			readMeta.ModifiedAt = meta.ModifiedAt
		}
		meta = readMeta
	}
	return meta, true, nil
}

func usableETag(settings Settings, tag string) bool {
	if strongETag(tag) {
		return true
	}
	address, _ := url.Parse(settings.WebDAVURL)
	// 坚果云 GET 返回不带引号的实体标签，If-Match 接受此原值。
	return address != nil && strings.EqualFold(address.Hostname(), "dav.jianguoyun.com") && tag != "" && !strings.HasPrefix(tag, "W/")
}

func strongETag(tag string) bool {
	return len(tag) >= 2 && strings.HasPrefix(tag, `"`) && strings.HasSuffix(tag, `"`)
}

// 使用 HTTP 条件写入，先读后盲写不能保护跨设备修改。
func writeBackup(ctx context.Context, d Data, expected string, checkExisting bool) (string, error) {
	if checkExisting {
		meta, exists, err := backupMetadata(ctx, d.Settings)
		if err != nil {
			return "", err
		}
		if exists {
			return meta.ETag, ErrBackupConflict
		}
	}
	if expected != "" && !usableETag(d.Settings, expected) {
		return "", fmt.Errorf("WebDAV 未提供可用于覆盖保护的 ETag，无法安全上传")
	}
	body, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return "", err
	}
	headers := map[string]string{"Content-Type": "application/json"}
	if expected == "" {
		headers["If-None-Match"] = "*"
	} else {
		headers["If-Match"] = expected
	}
	response, err := backupRequest(ctx, d.Settings, http.MethodPut, body, headers)
	if err != nil {
		return "", err
	}
	status := response.StatusCode
	meta := remoteMetadata(response)
	response.Body.Close()
	if (status == 404 || status == 409) && expected == "" {
		client, filename, err := webDAVClient(d.Settings)
		if err != nil {
			return "", err
		}
		// 目录创建交给成熟 WebDAV 库；重试仍然带条件头。
		if err = client.MkdirAll(path.Dir(filename), 0700); err != nil {
			return "", err
		}
		response, err = backupRequest(ctx, d.Settings, http.MethodPut, body, headers)
		if err != nil {
			return "", err
		}
		status = response.StatusCode
		meta = remoteMetadata(response)
		response.Body.Close()
	}
	if status == 412 {
		latest, _, err := backupMetadata(ctx, d.Settings)
		if err != nil {
			return "", err
		}
		return latest.ETag, ErrBackupConflict
	}
	if status < 200 || status >= 300 {
		return "", fmt.Errorf("WebDAV 备份失败: HTTP %d", status)
	}
	if !usableETag(d.Settings, meta.ETag) {
		// 有些服务器 PUT 不返回 ETag，读取实际内容核实后记录其版本。
		response, err = backupRequest(ctx, d.Settings, http.MethodGet, nil, nil)
		if err != nil {
			return "", err
		}
		actual, readErr := io.ReadAll(response.Body)
		response.Body.Close()
		meta = remoteMetadata(response)
		if readErr != nil {
			return "", readErr
		}
		if response.StatusCode != 200 || !bytes.Equal(actual, body) {
			return meta.ETag, ErrBackupConflict
		}
		if !usableETag(d.Settings, meta.ETag) {
			return "", fmt.Errorf("备份已上传，但 WebDAV 未提供可用于覆盖保护的 ETag；后续上传暂停")
		}
	}
	return meta.ETag, nil
}

func readBackup(ctx context.Context, settings Settings) (Data, remoteBackup, error) {
	response, err := backupRequest(ctx, settings, http.MethodGet, nil, nil)
	if err != nil {
		return Data{}, remoteBackup{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return Data{}, remoteBackup{}, fmt.Errorf("WebDAV 下载失败: HTTP %d", response.StatusCode)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return Data{}, remoteBackup{}, err
	}
	d, err := parseData(body)
	if err != nil {
		return Data{}, remoteBackup{}, fmt.Errorf("远端备份无效: %w", err)
	}
	meta := remoteMetadata(response)
	if meta.ModifiedAt == nil {
		latest, exists, err := backupMetadata(ctx, settings)
		if err != nil {
			return Data{}, remoteBackup{}, err
		}
		if !exists || latest.ETag != meta.ETag {
			return Data{}, remoteBackup{}, ErrBackupConflict
		}
		meta.ModifiedAt = latest.ModifiedAt
	}
	return d, meta, nil
}
