package tray

import (
	"fmt"
	"runtime"
	"time"

	"github.com/energye/systray"
)

func Start(icon []byte, show, quit func()) (func(), error) {
	ready := make(chan struct{})
	done := make(chan struct{})
	go func() {
		// Win32 窗口和消息循环必须属于同一系统线程。
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer close(done)
		systray.Run(func() {
			configure(icon, show, quit)
			close(ready)
		}, nil)
	}()
	select {
	case <-ready:
		return func() { systray.Quit(); <-done }, nil
	case <-time.After(5 * time.Second):
		systray.Quit()
		return nil, fmt.Errorf("初始化 Windows 托盘失败")
	}
}
