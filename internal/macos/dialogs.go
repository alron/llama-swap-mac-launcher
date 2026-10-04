// Package macos holds the few native macOS calls the app needs that
// systray doesn't provide: dialogs, the Preferences dialog, the About
// panel, launch at login and the menu-bar title. Each .go file's
// Objective-C is in the .m file of the same name.
//
// Every function can be called from any goroutine. The dialogs block until
// the user dismisses them; other calls meanwhile, from other goroutines,
// don't wait for that.
package macos

/*
#cgo CFLAGS: -fobjc-arc
#cgo LDFLAGS: -framework AppKit
#include <stdlib.h>
#include "dialogs.h"
*/
import "C"

import (
	"strings"
	"time"
	"unsafe"
)

// SaveFile shows a Save panel, starting in dir with name filled in, and
// returns the path chosen, or "" if the user cancelled.
func SaveFile(title, dir, name string) string {
	ct, cd, cn := C.CString(title), C.CString(dir), C.CString(name)
	defer C.free(unsafe.Pointer(ct))
	defer C.free(unsafe.Pointer(cd))
	defer C.free(unsafe.Pointer(cn))
	p := C.lsl_save_file(ct, cd, cn)
	if p == nil {
		return ""
	}
	defer C.free(unsafe.Pointer(p))
	return C.GoString(p)
}

// Alert shows a modal alert with an OK button.
func Alert(message, detail string) {
	AlertWithOutput(message, detail, nil)
}

// AlertWithOutput is Alert with a program's output, such as the last lines
// of llama-swap's, shown under the message in a scrollable box.
func AlertWithOutput(message, detail string, output []string) {
	cmsg := C.CString(message)
	defer C.free(unsafe.Pointer(cmsg))
	cdetail := C.CString(detail)
	defer C.free(unsafe.Pointer(cdetail))
	coutput := C.CString(strings.Join(output, "\n"))
	defer C.free(unsafe.Pointer(coutput))
	C.lsl_alert(cmsg, cdetail, coutput)
}

// Confirm shows a modal alert with two buttons, ok (the default) and
// cancel, and reports whether the user chose ok.
func Confirm(message, detail, ok, cancel string) bool {
	cmsg := C.CString(message)
	defer C.free(unsafe.Pointer(cmsg))
	cdetail := C.CString(detail)
	defer C.free(unsafe.Pointer(cdetail))
	cok := C.CString(ok)
	defer C.free(unsafe.Pointer(cok))
	ccancel := C.CString(cancel)
	defer C.free(unsafe.Pointer(ccancel))
	return C.lsl_confirm(cmsg, cdetail, cok, ccancel) != 0
}

// CloseDialogs is for quitting. It closes every open dialog, as if each were
// cancelled, so the code waiting on it carries on, and any dialog shown
// after it returns at once as cancelled, so that code can't open another.
// It waits up to timeout for them to close, and reports whether they did.
func CloseDialogs(timeout time.Duration) bool {
	return C.lsl_close_dialogs(C.double(timeout.Seconds())) != 0
}
