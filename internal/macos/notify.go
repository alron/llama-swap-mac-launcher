package macos

/*
#cgo CFLAGS: -fobjc-arc
#cgo LDFLAGS: -framework UserNotifications -framework Foundation
#include <stdlib.h>
#include "notify.h"
*/
import "C"

import (
	"encoding/json"
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

// RemoveAllNotifications removes all of the app's notifications.
func RemoveAllNotifications() {
	C.lsl_notify_remove_all()
}

// NextNotificationResponse waits for the next click on a notification.
func NextNotificationResponse() NotificationResponse {
	c := C.lsl_notify_next_response()
	defer C.free(unsafe.Pointer(c))
	return parseResponse(C.GoString(c))
}

// parseResponse reads notify.m's "<identifier>\n<action>". It splits at the
// last newline: the action ("default", or a button's, such as "unload")
// never has one, but an identifier made from a model's ID could (TODO item
// 84).
func parseResponse(s string) NotificationResponse {
	i := strings.LastIndexByte(s, '\n')
	if i < 0 {
		return NotificationResponse{ID: s}
	}
	return NotificationResponse{ID: s[:i], Action: s[i+1:]}
}

// NotificationPermission is what macOS allows the app's notifications, as
// set in System Settings → Notifications.
type NotificationPermission struct {
	// Authorization is "not determined" (never asked), "denied",
	// "authorized", "provisional" or "ephemeral".
	Authorization string `json:"authorization"`
	// Alerts, Sound, NotificationCentre and TimeSensitive are "enabled",
	// "disabled" or "not supported".
	Alerts             string `json:"alerts"`
	Sound              string `json:"sound"`
	NotificationCentre string `json:"notificationCentre"`
	TimeSensitive      string `json:"timeSensitive"`
	// Style is "none", "banner" (Temporary) or "alert" (Persistent).
	Style string `json:"style"`
}

// Shown reports whether notifications the app posts are shown at all.
func (p NotificationPermission) Shown() bool {
	return (p.Authorization == "authorized" || p.Authorization == "provisional") && p.Alerts == "enabled"
}

// Blocked reports whether the user has turned the app's notifications off,
// as opposed to never having been asked.
func (p NotificationPermission) Blocked() bool {
	return p.Authorization == "denied" || (p.Authorization != "not determined" && p.Alerts == "disabled")
}

// NotificationSettings returns what macOS allows the app's notifications.
func NotificationSettings() NotificationPermission {
	c := C.lsl_notify_settings()
	defer C.free(unsafe.Pointer(c))
	var p NotificationPermission
	_ = json.Unmarshal([]byte(C.GoString(c)), &p) // the shim writes it; empty if it ever doesn't parse
	return p
}
