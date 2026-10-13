package autostart

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/ini.v1"
	"howett.net/plist"
)

func TestRegistrationFilesEnableDisableAndErrors(t *testing.T) {
	for _, format := range []struct {
		name string
		set  func(*Manager, bool) error
		read func(*Manager) (bool, error)
	}{
		{"plist", (*Manager).setPlist, (*Manager).plistEnabled},
		{"desktop", (*Manager).setDesktop, (*Manager).desktopEnabled},
	} {
		t.Run(format.name, func(t *testing.T) {
			m := &Manager{path: filepath.Join(t.TempDir(), "autostart", "codex-deck."+format.name), executable: "/Applications/Codex Deck & 50%/codex-deck"}
			if enabled, err := format.read(m); err != nil || enabled {
				t.Fatalf("缺少注册项时应默认关闭: %v, %v", enabled, err)
			}
			if err := format.set(m, true); err != nil {
				t.Fatal(err)
			}
			if enabled, err := format.read(m); err != nil || !enabled {
				t.Fatalf("未读取实际注册项: %v, %v", enabled, err)
			}
			data, err := os.ReadFile(m.path)
			if err != nil {
				t.Fatal(err)
			}
			if format.name == "plist" {
				var job map[string]any
				if _, err := plist.Unmarshal(data, &job); err != nil {
					t.Fatal(err)
				}
				args := job["ProgramArguments"].([]any)
				if args[0] != m.executable || args[1] != Flag || job["KeepAlive"] != nil {
					t.Fatal("启动路径、参数或退出后重启规则错误")
				}
			} else {
				entry, err := ini.LoadSources(desktopOptions, data)
				if err != nil {
					t.Fatal(err)
				}
				if entry.Section("Desktop Entry").Key("Exec").Value() != `"/Applications/Codex Deck & 50%%/codex-deck" --autostart` {
					t.Fatal("带空格路径或百分号未按桌面规范转义")
				}
			}
			if err := os.WriteFile(m.path, []byte("invalid"), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := format.read(m); err == nil {
				t.Fatal("损坏注册项未明确报错")
			}
			if err := format.set(m, false); err != nil {
				t.Fatal(err)
			}
			if enabled, err := format.read(m); err != nil || enabled {
				t.Fatalf("关闭后仍有注册项: %v, %v", enabled, err)
			}
			if err := format.set(m, false); err != nil {
				t.Fatal(err)
			}
			m.path = t.TempDir()
			if err := format.set(m, true); err == nil {
				t.Fatal("无法替换目录时未暴露写入错误")
			}
		})
	}
}

func TestDisabledRegistrationFiles(t *testing.T) {
	m := &Manager{path: filepath.Join(t.TempDir(), "app.plist")}
	data, err := plist.Marshal(launchAgent{Label: label, ProgramArguments: []string{"app", Flag}, RunAtLoad: true, Disabled: true}, plist.XMLFormat)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(m.path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if enabled, err := m.plistEnabled(); err != nil || enabled {
		t.Fatal("忽略 LaunchAgent 禁用标记")
	}
	for _, key := range []string{"Hidden=true", "X-GNOME-Autostart-enabled=false"} {
		if err := os.WriteFile(m.path, []byte("[Desktop Entry]\nType=Application\nExec=app --autostart\n"+key+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
		if enabled, err := m.desktopEnabled(); err != nil || enabled {
			t.Fatalf("忽略桌面禁用标记: %s", key)
		}
	}
}

func TestIsLaunch(t *testing.T) {
	if IsLaunch(nil) || IsLaunch([]string{"/tmp/--autostart/app"}) || !IsLaunch([]string{Flag}) {
		t.Fatal("自动启动与手动启动来源识别错误")
	}
}
