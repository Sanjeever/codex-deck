package tray

import (
	"fmt"
	"os"
	"slices"

	"github.com/energye/systray"
	"github.com/godbus/dbus/v5"
)

func Start(icon []byte, actions Actions) (*Controller, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, fmt.Errorf("连接桌面会话失败: %w", err)
	}
	defer conn.Close()
	watcher := conn.Object("org.kde.StatusNotifierWatcher", "/StatusNotifierWatcher")
	value, err := watcher.GetProperty("org.kde.StatusNotifierWatcher.IsStatusNotifierHostRegistered")
	if err != nil {
		return nil, fmt.Errorf("当前桌面不支持系统托盘: %w", err)
	}
	var available bool
	if err := value.Store(&available); err != nil {
		return nil, err
	}
	if !available {
		return nil, fmt.Errorf("当前桌面没有运行托盘宿主")
	}
	// 提前设置图标和菜单，避免注册时导出空内容。
	update := configure(icon, actions)
	start, stop := systray.RunWithExternalLoop(nil, nil)
	start()
	name := fmt.Sprintf("org.kde.StatusNotifierItem-%d-1", os.Getpid())
	var owner string
	err = conn.BusObject().Call("org.freedesktop.DBus.GetNameOwner", 0, name).Store(&owner)
	if err == nil {
		value, err = watcher.GetProperty("org.kde.StatusNotifierWatcher.RegisteredStatusNotifierItems")
	}
	var items []string
	if err == nil {
		err = value.Store(&items)
	}
	if err == nil && !slices.Contains(items, owner+"/StatusNotifierItem") {
		err = fmt.Errorf("托盘宿主未注册 Codex Deck 图标")
	}
	if err != nil {
		stop()
		return nil, fmt.Errorf("注册系统托盘失败: %w", err)
	}
	return &Controller{Update: update, Stop: stop}, nil
}
