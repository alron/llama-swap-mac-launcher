package macos

/*
#cgo CFLAGS: -fobjc-arc
#cgo LDFLAGS: -framework AppKit
#include <stdlib.h>
#include "statusbar.h"
*/
import "C"

import "unsafe"

// Color is a colour for the menu-bar title. All but ColorAlert follow the
// menu bar's light or dark look.
type Color int32 // the size of a C int, which the shim takes

const (
	ColorNormal Color = iota // the menu bar's normal text colour
	ColorBusy                // grey
	ColorFaded               // the same grey; used for the letters too, it reads as faded
	ColorAlert               // red
)

// SetMenuBarTitle shows left, an arrow, then right in the menu bar, with the
// letters in letters and the arrow in arrow. It reports false if it couldn't
// find the menu-bar item.
func SetMenuBarTitle(left, right string, letters, arrow Color) bool {
	cleft := C.CString(left)
	defer C.free(unsafe.Pointer(cleft))
	cright := C.CString(right)
	defer C.free(unsafe.Pointer(cright))
	return C.lsl_set_menu_bar_title(cleft, cright, C.int(letters), C.int(arrow)) != 0
}

// MenuWorksDuringDialogs lets clicks on the menu's items through while a
// dialog is open; macOS otherwise drops them. It reports false if it
// couldn't, which would mean systray's internals changed.
func MenuWorksDuringDialogs() bool {
	return C.lsl_menu_works_when_modal() != 0
}
