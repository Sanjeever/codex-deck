package autostart

import (
	"fmt"
	"os"
	"testing"

	"golang.org/x/sys/windows/registry"
)

func TestWindowsRegistrationUsesIsolatedKey(t *testing.T) {
	path := fmt.Sprintf(`Software\CodexDeckTests\%s-%d`, t.Name(), os.Getpid())
	m := &Manager{path: path, executable: `C:\Program Files\Codex Deck\codex-deck.exe`}
	if enabled, err := m.Enabled(); err != nil || enabled {
		t.Fatalf("缺少注册项时应默认关闭: %v, %v", enabled, err)
	}
	if err := m.Set(true); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := registry.DeleteKey(registry.CURRENT_USER, path); err != nil {
			t.Error(err)
		}
	})
	key, err := registry.OpenKey(registry.CURRENT_USER, path, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		t.Fatal(err)
	}
	defer key.Close()
	command, _, err := key.GetStringValue(valueName)
	if err != nil || command != `"C:\Program Files\Codex Deck\codex-deck.exe" --autostart` {
		t.Fatalf("含空格启动命令错误: %q, %v", command, err)
	}
	if enabled, err := m.Enabled(); err != nil || !enabled {
		t.Fatalf("注册成功后开关仍关闭: %v, %v", enabled, err)
	}
	if err := key.SetStringValue("other_app", "untouched"); err != nil {
		t.Fatal(err)
	}
	if err := m.Set(false); err != nil {
		t.Fatal(err)
	}
	if enabled, err := m.Enabled(); err != nil || enabled {
		t.Fatalf("关闭后未移除注册项: %v, %v", enabled, err)
	}
	if other, _, err := key.GetStringValue("other_app"); err != nil || other != "untouched" {
		t.Fatal("修改其他启动项")
	}
	if err := m.Set(false); err != nil {
		t.Fatal(err)
	}
	if err := key.SetDWordValue(valueName, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Enabled(); err == nil {
		t.Fatal("损坏注册项未明确报错")
	}
}
