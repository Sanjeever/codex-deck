package main

import (
	"context"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/sys/windows"
)

func prepareHiddenWindow(ctx context.Context) error {
	var info windows.StartupInfo
	if err := windows.GetStartupInfo(&info); err != nil {
		return err
	}
	// Windows 会将隐藏启动信息用于首次 ShowWindow，先以隐藏调用消耗它。
	if info.Flags&windows.STARTF_USESHOWWINDOW != 0 && info.ShowWindow == windows.SW_HIDE {
		runtime.WindowHide(ctx)
	}
	return nil
}
