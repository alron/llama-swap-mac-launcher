package macos

/*
#cgo CFLAGS: -fobjc-arc
#cgo LDFLAGS: -framework LocalAuthentication -framework Foundation
#include <stdlib.h>
#include "auth.h"
*/
import "C"

import (
	"errors"
	"unsafe"
)

// ErrNotConfirmed is ConfirmOwner's answer when the person cancelled.
var ErrNotConfirmed = errors.New("it wasn't confirmed")

// ConfirmOwner asks the person at the Mac to confirm reason, with Touch ID,
// a paired Apple Watch or their login password (TODO item 51), and waits.
// macOS shows reason as "<the app> is trying to <reason>." It returns nil
// once confirmed; ErrNotConfirmed if they cancelled; otherwise why it
// couldn't be confirmed (a wrong finger too often, no password set).
// Call it from a goroutine, never the main thread.
func ConfirmOwner(reason string) error {
	c := cString(reason)
	defer C.free(unsafe.Pointer(c))
	var cerr *C.char
	r := C.lsl_confirm_owner(c, &cerr)
	if cerr != nil {
		defer C.free(unsafe.Pointer(cerr))
	}
	switch r {
	case 0:
		return nil
	case 1:
		return ErrNotConfirmed
	}
	if cerr == nil {
		return errors.New("it couldn't be confirmed")
	}
	return errors.New(C.GoString(cerr))
}

// cancelConfirmOwner ends a confirmation in progress, as cancelled.
func cancelConfirmOwner() { C.lsl_cancel_confirm_owner() }
