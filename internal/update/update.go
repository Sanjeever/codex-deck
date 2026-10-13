package update

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"sync"
	"time"

	"golang.org/x/mod/semver"
)

const Repository = "Sanjeever/codex-deck"

type State struct {
	Phase      string     `json:"phase"`
	Error      string     `json:"error"`
	Version    string     `json:"version"`
	Notes      string     `json:"notes"`
	URL        string     `json:"url"`
	CheckedAt  *time.Time `json:"checked_at"`
	Downloaded int64      `json:"downloaded"`
	Total      int64      `json:"total"`
}

type Release struct {
	Tag        string `json:"tag_name"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
	Body       string `json:"body"`
	URL        string `json:"html_url"`
	Assets     []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
		Size int64  `json:"size"`
	} `json:"assets"`
}

type Manager struct {
	workers    sync.WaitGroup
	stopping   bool
	mu         sync.Mutex
	state      State
	assetURL   string
	assetName  string
	cancel     context.CancelFunc
	parentPipe interface{ Close() error }
	changed    func()
	Client     *http.Client
	Endpoint   string
	Version    string
	Platform   string
}

func New(version string, changed func()) *Manager {
	platform := runtime.GOOS + "_" + runtime.GOARCH
	if runtime.GOOS == "darwin" {
		platform = "darwin_universal"
	}
	return &Manager{Version: version, Platform: platform, Client: &http.Client{}, Endpoint: "https://api.github.com/repos/" + Repository + "/releases/latest", changed: changed}
}

func (m *Manager) State() State { m.mu.Lock(); defer m.mu.Unlock(); return m.state }
func (m *Manager) publish(change func(*State)) {
	m.mu.Lock()
	change(&m.state)
	m.mu.Unlock()
	if m.changed != nil {
		m.changed()
	}
}
func (m *Manager) fail(err error) { m.publish(func(s *State) { s.Phase = ""; s.Error = err.Error() }) }

func (m *Manager) Check(ctx context.Context) error {
	m.mu.Lock()
	if m.state.Phase != "" {
		m.mu.Unlock()
		return fmt.Errorf("更新任务正在进行")
	}
	m.state.Phase = "checking"
	m.state.Error = ""
	m.mu.Unlock()
	if m.changed != nil {
		m.changed()
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, m.Endpoint, nil)
	if err == nil {
		request.Header.Set("Accept", "application/vnd.github+json")
		request.Header.Set("User-Agent", "Codex-Deck/"+m.Version)
		var response *http.Response
		response, err = m.Client.Do(request)
		if err == nil {
			defer response.Body.Close()
			if response.StatusCode == http.StatusNotFound {
				m.publish(func(s *State) { now := time.Now(); *s = State{CheckedAt: &now} })
				return nil
			}
			if response.StatusCode != http.StatusOK {
				err = fmt.Errorf("检查更新失败：HTTP %d", response.StatusCode)
			} else {
				var release Release
				if err = json.NewDecoder(response.Body).Decode(&release); err == nil {
					err = m.selectRelease(release)
				}
			}
		}
	}
	if err != nil {
		m.fail(err)
	}
	return err
}

func (m *Manager) selectRelease(release Release) error {
	if !semver.IsValid(release.Tag) || !semver.IsValid("v"+m.Version) {
		return fmt.Errorf("更新版本格式无效")
	}
	if release.Draft || release.Prerelease || semver.Prerelease(release.Tag) != "" {
		return fmt.Errorf("更新源不是正式版本")
	}
	now := time.Now()
	if semver.Compare(release.Tag, "v"+m.Version) <= 0 {
		m.publish(func(s *State) { *s = State{CheckedAt: &now} })
		return nil
	}
	version := release.Tag[1:]
	ext := ".zip"
	if m.Platform == "linux_amd64" {
		ext = ".tar.gz"
	}
	name := "codex-deck_" + version + "_" + m.Platform + ext
	for _, asset := range release.Assets {
		if asset.Name != name {
			continue
		}
		if err := trustedDownload(asset.URL); err != nil {
			return err
		}
		m.mu.Lock()
		m.assetURL, m.assetName = asset.URL, name
		m.mu.Unlock()
		m.publish(func(s *State) {
			*s = State{Version: version, Notes: release.Body, URL: release.URL, CheckedAt: &now, Total: asset.Size}
		})
		return nil
	}
	return fmt.Errorf("新版本缺少 %s 安装包", m.Platform)
}

func (m *Manager) Cancel() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cancel != nil {
		m.cancel()
	}
}

func (m *Manager) Stop() {
	m.mu.Lock()
	m.stopping = true
	if m.cancel != nil {
		m.cancel()
	}
	m.mu.Unlock()
	m.workers.Wait()
}

func (m *Manager) ReleaseParent() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.parentPipe != nil {
		return m.parentPipe.Close()
	}
	return nil
}
