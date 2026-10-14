package tray

/*
#cgo CFLAGS: -x objective-c -fblocks
#cgo LDFLAGS: -framework Cocoa
#include <stdbool.h>
#include <stdlib.h>
bool codexDeckTrayStart(const unsigned char *icon, int length);
void codexDeckTrayUpdate(const char *menu);
void codexDeckTrayStop(void);
*/
import "C"

import (
	"encoding/json"
	"fmt"
	"unsafe"
)

var callbacks Actions

func Start(icon []byte, actions Actions) (*Controller, error) {
	callbacks = actions
	if !bool(C.codexDeckTrayStart((*C.uchar)(unsafe.Pointer(&icon[0])), C.int(len(icon)))) {
		return nil, fmt.Errorf("创建 macOS 菜单栏图标失败")
	}
	return &Controller{Update: func(menu Menu) {
		data, err := json.Marshal(menu)
		if err != nil {
			panic(err)
		}
		payload := C.CString(string(data))
		defer C.free(unsafe.Pointer(payload))
		C.codexDeckTrayUpdate(payload)
	}, Stop: func() { C.codexDeckTrayStop() }}, nil
}

//export codexDeckTrayShow
func codexDeckTrayShow() { go callbacks.Show() }

//export codexDeckTrayQuit
func codexDeckTrayQuit() { go callbacks.Quit() }

//export codexDeckTrayRefresh
func codexDeckTrayRefresh() { go callbacks.Refresh() }

//export codexDeckTraySwitch
func codexDeckTraySwitch(id *C.char) {
	accountID := C.GoString(id)
	go callbacks.Switch(accountID)
}
