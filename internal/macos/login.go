package macos

/*
#cgo CFLAGS: -fobjc-arc
#cgo LDFLAGS: -framework ServiceManagement -framework Foundation
#include <stdlib.h>
#include "login.h"
*/
import "C"

import (
	"errors"
	"unsafe"
)

// LoginItemStatus is SMAppServiceStatus: whether the app opens at login.
type LoginItemStatus int

const (
	LoginItemNotRegistered LoginItemStatus = iota
	LoginItemEnabled
	// LoginItemRequiresApproval means the app is registered, but the user
	// has to allow it in System Settings → General → Login Items. It's also
	// what the status becomes if the user turns it off there.
	LoginItemRequiresApproval
	LoginItemNotFound
)

// LoginItem returns whether the app is set to open at login.
func LoginItem() LoginItemStatus {
	return LoginItemStatus(C.lsl_login_item_status())
}

// SetLoginItem sets or clears opening the app at login.
func SetLoginItem(on bool) error {
	enable := C.int(0)
	if on {
		enable = 1
	}
	if msg := C.lsl_login_item_set(enable); msg != nil {
		defer C.free(unsafe.Pointer(msg))
		return errors.New(C.GoString(msg))
	}
	return nil
}

// OpenLoginItemsSettings opens System Settings at Login Items.
func OpenLoginItemsSettings() {
	C.lsl_open_login_items_settings()
}
