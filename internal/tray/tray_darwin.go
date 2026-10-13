package tray

/*
#cgo CFLAGS: -x objective-c -fblocks
#cgo LDFLAGS: -framework Cocoa
#include <stdbool.h>
bool codexDeckTrayStart(const unsigned char *icon, int length);
void codexDeckTrayStop(void);
*/
import "C"

import (
	"fmt"
	"unsafe"
)

var showWindow, quitApp func()

func Start(icon []byte, show, quit func()) (func(), error) {
	showWindow, quitApp = show, quit
	if !bool(C.codexDeckTrayStart((*C.uchar)(unsafe.Pointer(&icon[0])), C.int(len(icon)))) {
		return nil, fmt.Errorf("创建 macOS 菜单栏图标失败")
	}
	return func() { C.codexDeckTrayStop() }, nil
}

//export codexDeckTrayShow
func codexDeckTrayShow() { go showWindow() }

//export codexDeckTrayQuit
func codexDeckTrayQuit() { go quitApp() }
