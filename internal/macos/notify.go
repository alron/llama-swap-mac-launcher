package macos

/*
#cgo CFLAGS: -fobjc-arc
#cgo LDFLAGS: -framework UserNotifications -framework Foundation
#include <stdlib.h>
#include "notify.h"
*/
import "C"

import (
	"errors"
	"strings"
	"unsafe"
)

// NotificationResponse is a click on one of the app's notifications.
type NotificationResponse struct {
	ID string // the notification's
	// Action is "default" for a click on the notification itself, or the
	// button's identifier ("unload").
	Action string
}

// SetUpNotifications makes the app the receiver of clicks on its
// notifications, and shows banners while it's active. Call it once, early.
func SetUpNotifications() {
	C.lsl_notify_setup()
}

// AuthorizeNotifications asks macOS for permission to post notifications,
// which shows the prompt the first time, and reports whether they're
// allowed.
func AuthorizeNotifications() (bool, error) {
	var cerr *C.char
	ok := C.lsl_notify_authorize(&cerr) == 1
	if cerr != nil {
		defer C.free(unsafe.Pointer(cerr))
		return ok, errors.New(C.GoString(cerr))
	}
	return ok, nil
}

// Notify posts a notification, replacing any with the same id. A
// time-sensitive one gets through a Focus, if the app is signed with the
// entitlement; without it, macOS posts it at the ordinary level.
func Notify(id, title, body, category string, timeSensitive bool) error {
	cs := []*C.char{cString(id), cString(title), cString(body), cString(category)}
	defer func() {
		for _, c := range cs {
			C.free(unsafe.Pointer(c))
		}
	}()
	ts := C.int(0)
	if timeSensitive {
		ts = 1
	}
	if cerr := C.lsl_notify_post(cs[0], cs[1], cs[2], cs[3], ts); cerr != nil {
		defer C.free(unsafe.Pointer(cerr))
		return errors.New(C.GoString(cerr))
	}
	return nil
}

// RemoveNotification removes the notification with id from Notification
// Centre, or before it's shown.
func RemoveNotification(id string) {
	c := cString(id)
	defer C.free(unsafe.Pointer(c))
	C.lsl_notify_remove(c)
}

// NextNotificationResponse waits for the next click on a notification.
func NextNotificationResponse() NotificationResponse {
	c := C.lsl_notify_next_response()
	defer C.free(unsafe.Pointer(c))
	id, action, _ := strings.Cut(C.GoString(c), "\n")
	return NotificationResponse{ID: id, Action: action}
}

// NotificationSettings returns what macOS allows the app, as JSON: whether
// it may post, in what style, and whether time-sensitive ones get through.
func NotificationSettings() string {
	c := C.lsl_notify_settings()
	defer C.free(unsafe.Pointer(c))
	return C.GoString(c)
}
