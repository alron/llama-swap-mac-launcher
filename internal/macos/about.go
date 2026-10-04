package macos

/*
#cgo CFLAGS: -fobjc-arc
#cgo LDFLAGS: -framework AppKit
#include <stdlib.h>
#include "about.h"
*/
import "C"

import (
	"strings"
	"unsafe"
)

// ShowAbout shows the standard About panel: the app's icon, name and
// version, then the credits, one line each, and link as a clickable
// address. The panel stays open until it's closed; ShowAbout doesn't wait.
func ShowAbout(version string, credits []string, link string) {
	cversion := C.CString(version)
	defer C.free(unsafe.Pointer(cversion))
	ccredits := C.CString(strings.Join(credits, "\n"))
	defer C.free(unsafe.Pointer(ccredits))
	clink := C.CString(link)
	defer C.free(unsafe.Pointer(clink))
	C.lsl_show_about(cversion, ccredits, clink)
}
