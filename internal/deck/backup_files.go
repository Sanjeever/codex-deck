package deck

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/studio-b12/gowebdav"
)

const backupTimeLayout = "20060102T150405.000Z"

type BackupFile struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Hostname   string     `json:"hostname"`
	CreatedAt  *time.Time `json:"created_at" ts_type:"string | null"`
	ModifiedAt *time.Time `json:"modified_at" ts_type:"string | null"`
	Size       int64      `json:"size"`
	Legacy     bool       `json:"legacy"`
}

type BackupMetadata struct {
	CreatedAt  time.Time  `json:"created_at"`
	Hostname   string     `json:"hostname"`
	ModifiedAt *time.Time `json:"-"`
}

type backupSnapshot struct {
	Data
	Metadata BackupMetadata `json:"backup_metadata"`
}

func snapshotName(at time.Time, hostname string) string {
	host := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == '.' {
			return r
		}
		return '-'
	}, hostname)
	return "codex-deck_" + at.UTC().Format(backupTimeLayout) + "_" + host + "_" + uuid.NewString()[:8] + ".json"
}

func snapshotFile(name string) (BackupFile, bool) {
	if path.Base(name) != name || !strings.HasPrefix(name, "codex-deck_") || !strings.HasSuffix(name, ".json") {
		return BackupFile{}, false
	}
	parts := strings.SplitN(strings.TrimPrefix(strings.TrimSuffix(name, ".json"), "codex-deck_"), "_", 2)
	if len(parts) != 2 {
		return BackupFile{}, false
	}
	at, err := time.Parse(backupTimeLayout, parts[0])
	end := strings.LastIndex(parts[1], "_")
	if err != nil || end <= 0 || len(parts[1][end+1:]) != 8 {
		return BackupFile{}, false
	}
	return BackupFile{ID: path.Join("codex-deck", name), Name: name, Hostname: parts[1][:end], CreatedAt: &at}, true
}

func writeSnapshot(ctx context.Context, d Data) (BackupFile, error) {
	client, _, err := webDAVClient(d.Settings)
	if err != nil {
		return BackupFile{}, err
	}
	hostname, err := os.Hostname()
	if err != nil {
		return BackupFile{}, fmt.Errorf("读取设备名称失败: %w", err)
	}
	at := time.Now().UTC()
	name := snapshotName(at, hostname)
	d.Network = nil
	// 快照来源描述当前上传设备，不继承恢复文件中的来源信息。
	body, err := json.MarshalIndent(backupSnapshot{Data: d, Metadata: BackupMetadata{CreatedAt: at, Hostname: hostname}}, "", "  ")
	if err != nil {
		return BackupFile{}, err
	}
	headers := map[string]string{"Content-Type": "application/json", "If-None-Match": "*"}
	filename := path.Join("codex-deck", name)
	response, err := backupFileRequest(ctx, d.Settings, filename, http.MethodPut, body, headers)
	if err != nil {
		return BackupFile{}, err
	}
	status := response.StatusCode
	response.Body.Close()
	if status == http.StatusNotFound || status == http.StatusConflict {
		client.SetTransport(backupContextTransport{ctx: ctx})
		if err := client.MkdirAll("codex-deck", 0700); err != nil {
			return BackupFile{}, err
		}
		response, err = backupFileRequest(ctx, d.Settings, filename, http.MethodPut, body, headers)
		if err != nil {
			return BackupFile{}, err
		}
		status = response.StatusCode
		response.Body.Close()
	}
	if status < 200 || status >= 300 {
		return BackupFile{}, fmt.Errorf("WebDAV 备份失败: HTTP %d", status)
	}
	return BackupFile{ID: filename, Name: name, Hostname: hostname, CreatedAt: &at, Size: int64(len(body))}, nil
}

// 列表读取目录元数据；账号凭据只在选中文件后下载。
func listBackupFiles(ctx context.Context, settings Settings) ([]BackupFile, error) {
	client, legacy, err := webDAVClient(settings)
	if err != nil {
		return nil, err
	}
	client.SetTransport(backupContextTransport{ctx: ctx})
	entries, err := client.ReadDir("codex-deck")
	if err != nil && !gowebdav.IsErrNotFound(err) {
		return nil, fmt.Errorf("读取备份列表失败: %w", err)
	}
	files := []BackupFile{}
	for _, entry := range entries {
		file, ok := snapshotFile(entry.Name())
		if entry.IsDir() || !ok {
			continue
		}
		file.Size = entry.Size()
		if at := entry.ModTime(); !at.IsZero() {
			file.ModifiedAt = &at
		}
		files = append(files, file)
	}
	entry, err := client.Stat(legacy)
	if err != nil && !gowebdav.IsErrNotFound(err) {
		return nil, fmt.Errorf("检查旧版备份失败: %w", err)
	}
	if err == nil && !entry.IsDir() {
		file := BackupFile{ID: legacy, Name: path.Base(legacy), Size: entry.Size(), Legacy: true}
		if at := entry.ModTime(); !at.IsZero() {
			file.ModifiedAt = &at
		}
		files = append(files, file)
	}
	sort.Slice(files, func(i, j int) bool {
		left, right := backupFileTime(files[i]), backupFileTime(files[j])
		if left.Equal(right) {
			return files[i].ID > files[j].ID
		}
		return left.After(right)
	})
	return files, nil
}

func backupFileTime(file BackupFile) time.Time {
	if file.CreatedAt != nil {
		return *file.CreatedAt
	}
	if file.ModifiedAt != nil {
		return *file.ModifiedAt
	}
	return time.Time{}
}

func readSnapshot(ctx context.Context, settings Settings, filename string) (Data, BackupMetadata, []byte, error) {
	response, err := backupFileRequest(ctx, settings, filename, http.MethodGet, nil, nil)
	if err != nil {
		return Data{}, BackupMetadata{}, nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return Data{}, BackupMetadata{}, nil, fmt.Errorf("WebDAV 下载失败: HTTP %d", response.StatusCode)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return Data{}, BackupMetadata{}, nil, err
	}
	d, err := parseBackupData(body)
	if err != nil {
		return Data{}, BackupMetadata{}, nil, fmt.Errorf("远端备份无效: %w", err)
	}
	var envelope struct {
		Metadata BackupMetadata `json:"backup_metadata"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return Data{}, BackupMetadata{}, nil, fmt.Errorf("备份来源信息无效: %w", err)
	}
	if at, err := http.ParseTime(response.Header.Get("Last-Modified")); err == nil {
		envelope.Metadata.ModifiedAt = &at
	}
	return d, envelope.Metadata, body, nil
}
