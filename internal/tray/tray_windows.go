package tray

import (
	"fmt"
	"runtime"
	"time"

	"github.com/energye/systray"
)

func Start(icon []byte, actions Actions) (*Controller, error) {
	ready := make(chan struct{})
	done := make(chan struct{})
	var update func(Menu)
	go func() {
		// Win32 窗口和消息循环必须属于同一系统线程。
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer close(done)
		systray.Run(func() {
			update = configure(icon, actions)
			close(ready)
		}, nil)
	}()
	select {
	case <-ready:
		return &Controller{Update: update, Stop: func() { systray.Quit(); <-done }}, nil
	case <-time.After(5 * time.Second):
		systray.Quit()
		return nil, fmt.Errorf("初始化 Windows 托盘失败")
	}
}
