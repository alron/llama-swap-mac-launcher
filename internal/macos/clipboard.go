package macos

/*
#cgo CFLAGS: -fobjc-arc
#cgo LDFLAGS: -framework AppKit
#include "clipboard.h"
*/
import "C"

// ClearCopiedSecret empties the clipboard if it still holds a key or secret
// variable copied in Settings, and nothing has been copied since. The app
// calls it as it quits; otherwise it happens after the delay set on the
// Tweaks tab.
func ClearCopiedSecret() {
	C.lsl_clear_copied()
}
