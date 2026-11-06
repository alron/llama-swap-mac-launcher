package macos

/*
#cgo CFLAGS: -fobjc-arc
#cgo LDFLAGS: -framework ServiceManagement -framework Metal -framework Foundation
#include <stdlib.h>
#include "wired.h"
*/
import "C"

import (
	"errors"
	"time"
	"unsafe"
)

// HelperStatus is the SMAppService status of the daemon with plist (as
// LoginItemStatus is the app's): not registered, enabled, waiting for the
// user's approval in System Settings, or not found in the bundle.
func HelperStatus(plist string) LoginItemStatus {
	c := cString(plist)
	defer C.free(unsafe.Pointer(c))
	return LoginItemStatus(C.lsl_helper_status(c))
}

// RegisterHelper registers the daemon with plist (or, with on false,
// unregisters it). Registered, it waits for the user's approval in System
// Settings → General → Login Items & Extensions.
func RegisterHelper(plist string, on bool) error {
	c := cString(plist)
	defer C.free(unsafe.Pointer(c))
	enable := C.int(0)
	if on {
		enable = 1
	}
	if cerr := C.lsl_helper_register(c, enable); cerr != nil {
		defer C.free(unsafe.Pointer(cerr))
		return errors.New(C.GoString(cerr))
	}
	return nil
}

// HelperReply is the helper's answer: whether it did what was asked (and
// if not, why), the user it runs as, the limit now and the one it keeps.
type HelperReply struct {
	OK     bool
	Error  string
	UID    int64
	Limit  int64
	Stored int64
}

// HelperRequest asks the helper at service, which must meet requirement,
// to do op ("info", or "set" with mb), waiting up to timeout for it. The
// error is for a request that didn't reach the helper or wasn't answered
// in time; one it refused comes back in the reply.
func HelperRequest(service, requirement, op string, mb int64, timeout time.Duration) (HelperReply, error) {
	cs, cr, co := cString(service), cString(requirement), cString(op)
	defer func() {
		for _, c := range []*C.char{cs, cr, co} {
			C.free(unsafe.Pointer(c))
		}
	}()
	var ok C.int
	var cerr *C.char
	var uid, limit, stored C.longlong
	if failed := C.lsl_helper_request(cs, cr, co, C.longlong(mb), C.double(timeout.Seconds()), &ok, &cerr, &uid, &limit, &stored); failed != nil {
		defer C.free(unsafe.Pointer(failed))
		return HelperReply{}, errors.New(C.GoString(failed))
	}
	defer C.free(unsafe.Pointer(cerr))
	return HelperReply{OK: ok == 1, Error: C.GoString(cerr), UID: int64(uid), Limit: int64(limit), Stored: int64(stored)}, nil
}

// GPUWorkingSetMB is how much memory Metal says the GPU should use, in MB:
// the GPU's wired limit, so with iogpu.wired_limit_mb at 0, macOS's
// default (79626 MB, about 78 GB, of a 96 GB Mac). Metal fixes it when the
// process first uses the GPU, so it's the limit as it was then. 0 if
// there's no GPU.
func GPUWorkingSetMB() int64 { return int64(C.lsl_gpu_working_set_mb()) }
