package macos

/*
#cgo CFLAGS: -fobjc-arc
#cgo LDFLAGS: -framework Security -framework Foundation
#include <stdlib.h>
#include "codesign.h"
*/
import "C"

import (
	"errors"
	"unsafe"
)

// CaptureSelfRequirement keeps the running app's designated requirement,
// for MatchesSelf. Call it at launch, before anything can replace the
// bundle. An ad-hoc build's requirement is its own exact code, which no
// other build meets.
func CaptureSelfRequirement() error {
	if cerr := C.lsl_capture_self_requirement(); cerr != nil {
		defer C.free(unsafe.Pointer(cerr))
		return errors.New(C.GoString(cerr))
	}
	return nil
}

// MatchesSelf checks that the app bundle at path is intact and signed as
// the running app is (its bundle ID and team), so it's safe to restart
// into (TODO item 65).
func MatchesSelf(path string) error {
	c := cString(path)
	defer C.free(unsafe.Pointer(c))
	if cerr := C.lsl_matches_self(c); cerr != nil {
		defer C.free(unsafe.Pointer(cerr))
		return errors.New(C.GoString(cerr))
	}
	return nil
}
