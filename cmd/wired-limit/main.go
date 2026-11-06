// wired-limit is Llama Swap Launcher's helper for the GPU's wired memory
// limit (TODO item 51): iogpu.wired_limit_mb, which lets the GPU wire more
// of the Mac's memory, needs root to set, and resets at every boot.
//
// launchd runs it as root, registered by the app through SMAppService
// (its plist is in the app's Contents/Library/LaunchDaemons) and approved
// by the user in System Settings. At every start, including boot, it
// applies the value it keeps; then it answers the app over XPC, and only
// the app: the service accepts no other code than the app's, signed by the
// same team. Whatever it's sent, it never leaves macOS less than 8 GB, and
// it refuses Macs with 32 GB or less.
package main

/*
#include <stdlib.h>
#include "xpc.h"
*/
import "C"

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"

	"github.com/alron/llama-swap-mac-launcher/internal/wired"
)

// Set by the Makefile: the app's bundle ID, and the team that signs both.
// The helper's launchd label and Mach service is wired.Label(client).
var (
	client = ""
	team   = ""
)

func logf(format string, args ...any) {
	// launchd sends stderr to the plist's StandardErrorPath.
	fmt.Fprintf(os.Stderr, "%s %s\n", time.Now().Format(time.RFC3339), fmt.Sprintf(format, args...))
}

func main() {
	if client == "" || team == "" {
		logf("built without its app or team; not starting")
		os.Exit(1)
	}
	label := wired.Label(client)
	// launchd's plist makes it root; anywhere else, it does nothing.
	if os.Geteuid() != 0 {
		logf("not running as root (uid %d); not starting", os.Geteuid())
		os.Exit(1)
	}
	logf("started as uid %d", os.Getuid())
	stored, err := readStored()
	if err != nil {
		logf("couldn't read the stored limit: %v", err)
	}
	if stored != 0 {
		// Early in boot, the setting may not exist yet: wait for it.
		if err := waitForSetting(time.Minute); err != nil {
			logf("%v", err)
		} else if err := apply(stored); err != nil {
			logf("couldn't apply the stored limit, %d MB: %v", stored, err)
		} else {
			logf("applied the stored limit, %d MB", stored)
		}
	} else if limit, err := currentLimit(); err == nil {
		logf("no limit stored; iogpu.wired_limit_mb is %d", limit)
	}

	// The app, signed by this team: anything else is refused by XPC itself.
	cname, creq := C.CString(label), C.CString(wired.AppRequirement(client, team))
	if cerr := C.lsl_helper_listen(cname, creq); cerr != nil {
		logf("couldn't listen on %s: %s", label, C.GoString(cerr))
		os.Exit(1)
	}
	for {
		var cop *C.char
		var mb C.longlong
		var hasMB C.int
		h := C.lsl_helper_next(&cop, &mb, &hasMB)
		op := C.GoString(cop)
		C.free(unsafe.Pointer(cop))
		answer(h, op, int64(mb), hasMB != 0)
	}
}

// answer handles one request from the app: "info", or "set" with mb,
// which must be there: a malformed request isn't macOS's default.
func answer(h unsafe.Pointer, op string, mb int64, hasMB bool) {
	var err error
	switch op {
	case "info":
	case "set":
		if !hasMB {
			err = errors.New("a set request without a limit")
		} else if err = check(mb); err == nil {
			if err = apply(mb); err == nil {
				// Applied, but not kept: in effect until the Mac restarts,
				// which the reply's limit shows and the error says.
				if serr := store(mb); serr != nil {
					err = fmt.Errorf("set to %d MB for now, but couldn't keep it for the next startup: %w", mb, serr)
				}
			}
		}
		if err != nil {
			logf("refused the app's limit of %d MB: %v", mb, err)
		} else {
			logf("set the limit to %d MB, as the app asked", mb)
		}
	default:
		err = fmt.Errorf("unknown request %q", op)
	}
	limit, _ := currentLimit()
	stored, _ := readStored()
	reply(h, err, limit, stored)
}

func reply(h unsafe.Pointer, err error, limit, stored int64) {
	ok, msg := C.int(1), ""
	if err != nil {
		ok, msg = 0, err.Error()
	}
	cmsg := C.CString(msg)
	defer C.free(unsafe.Pointer(cmsg))
	C.lsl_helper_reply(h, ok, cmsg, C.longlong(os.Getuid()), C.longlong(limit), C.longlong(stored))
}

// check enforces the bounds (wired.Check), whatever the app sent: 0 is
// macOS's default; otherwise at least 8 GB is left for macOS, on a Mac of
// more than 32 GB.
func check(mb int64) error {
	if mb == 0 {
		return nil
	}
	total, err := unix.SysctlUint64("hw.memsize")
	if err != nil {
		return fmt.Errorf("can't read the Mac's memory: %w", err)
	}
	return wired.Check(mb, int64(total>>20))
}

func apply(mb int64) error {
	if mb > 1<<31-1 {
		return fmt.Errorf("%d MB is too large", mb)
	}
	if e := C.lsl_set_wired_limit_mb(C.int(mb)); e != 0 {
		return fmt.Errorf("sysctl iogpu.wired_limit_mb: %w", syscall.Errno(e))
	}
	return nil
}

func currentLimit() (int64, error) {
	v, err := unix.SysctlUint32("iogpu.wired_limit_mb")
	return int64(v), err
}

// waitForSetting waits until iogpu.wired_limit_mb exists, which it may not
// yet early in boot.
func waitForSetting(max time.Duration) error {
	start := time.Now()
	deadline := start.Add(max)
	for {
		if _, err := currentLimit(); err == nil {
			if waited := time.Since(start); waited > 0 {
				logf("iogpu.wired_limit_mb was there after %s", waited.Round(time.Millisecond))
			}
			return nil
		} else if time.Now().After(deadline) {
			return fmt.Errorf("iogpu.wired_limit_mb wasn't there after %s: %w", max, err)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// The stored limit is root's, in a folder of its own, applied at every
// start; 0 or none means macOS's default.
func storedPath() string { return wired.StoredPath(client) }

// readStored reads it, and checks it against the bounds again.
func readStored() (int64, error) {
	mb, err := wired.Stored(client)
	if err != nil {
		return 0, err
	}
	if err := check(mb); err != nil {
		return 0, fmt.Errorf("%s holds %d: %w", storedPath(), mb, err)
	}
	return mb, nil
}

// store keeps mb for every start, or with 0 removes it. As root, it
// follows no symlinks: its folder must be root's own and not a link, and
// the file is opened without following one (O_NOFOLLOW), so a link planted
// there, should anything loosen /Library/Application Support, can't send
// root's write elsewhere. Removing a link removes only the link.
func store(mb int64) error {
	if mb == 0 {
		if err := os.Remove(storedPath()); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	dir := filepath.Dir(storedPath())
	switch fi, err := os.Lstat(dir); {
	case errors.Is(err, os.ErrNotExist):
		if err := os.Mkdir(dir, 0o755); err != nil { // #nosec G301 -- readable by all, so the app and llsl can show the limit; only root writes
			return err
		}
	case err != nil:
		return err
	case !fi.IsDir() || fi.Sys().(*syscall.Stat_t).Uid != 0 || fi.Mode().Perm()&0o022 != 0:
		return fmt.Errorf("%s isn't a folder only root can write", dir)
	}
	// 0644: the app and llsl read it, as the user, to show the limit.
	f, err := os.OpenFile(storedPath(), os.O_WRONLY|os.O_CREATE|os.O_TRUNC|syscall.O_NOFOLLOW, 0o644) // #nosec G302 G304 -- a limit, not a secret; the path is the helper's own
	if err != nil {
		return err
	}
	if fi, err := f.Stat(); err != nil || !fi.Mode().IsRegular() {
		_ = f.Close() // refused anyway
		return fmt.Errorf("%s isn't a plain file", storedPath())
	}
	if _, err := f.WriteString(strconv.FormatInt(mb, 10) + "\n"); err != nil {
		_ = f.Close() // the write's error is the one to report
		return err
	}
	return f.Close()
}
