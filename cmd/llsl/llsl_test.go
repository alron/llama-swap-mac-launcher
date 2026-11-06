package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alron/llama-swap-mac-launcher/internal/control"
	"github.com/alron/llama-swap-mac-launcher/internal/paths"
)

func TestLastLines(t *testing.T) {
	b := []byte("a\nb\nc\n")
	for n, want := range map[int]string{0: "", 1: "c\n", 2: "b\nc\n", 3: "a\nb\nc\n", 9: "a\nb\nc\n"} {
		if got := string(lastLines(b, n)); got != want {
			t.Errorf("lastLines(%d) = %q, want %q", n, got, want)
		}
	}
	if got := string(lastLines([]byte("a\nb"), 1)); got != "b" {
		t.Errorf("without a trailing newline: got %q", got)
	}
}

func TestExitCode(t *testing.T) {
	ready := &control.Status{State: "ready"}
	faulted := &control.Status{State: "ready", Models: []control.Model{{ID: "m", State: "ready", Faulted: true}}}
	tests := []struct {
		name     string
		resp     control.Response
		isStatus bool
		want     int
	}{
		{"ok", control.Response{OK: true, Status: ready}, false, exitOK},
		{"status ready", control.Response{OK: true, Status: ready}, true, exitOK},
		{"status starting", control.Response{OK: true, Status: &control.Status{State: "starting"}}, true, exitFailed},
		{"status faulted", control.Response{OK: true, Status: faulted}, true, exitFailed},
		{"invalid config", control.Response{Code: control.CodeInvalidConfig}, false, exitInvalidConfig},
		{"load failed", control.Response{Code: control.CodeLoadFailed}, false, exitLoadFailed},
		{"timeout", control.Response{Code: control.CodeTimeout}, false, exitTimeout},
		{"start failed", control.Response{Code: control.CodeStartFailed}, false, exitFailed},
		{"usage", control.Response{Code: control.CodeUsage}, false, exitUsage},
		{"status paused", control.Response{OK: true, Status: &control.Status{State: "paused", Paused: true}}, true, exitPaused},
		{"pause ok", control.Response{OK: true, Status: &control.Status{State: "paused", Paused: true}}, false, exitOK},
		{"load refused while paused", control.Response{Code: control.CodePaused}, false, exitPaused},
		{"busy", control.Response{Code: control.CodeBusy}, false, exitBusy},
	}
	for _, tt := range tests {
		if got := exitCode(tt.resp, tt.isStatus); got != tt.want {
			t.Errorf("%s: got %d, want %d", tt.name, got, tt.want)
		}
	}
}

func TestPlistBundleID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Info.plist")
	os.WriteFile(path, []byte(`<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0">
<dict>
	<key>CFBundleName</key>
	<string>Llama Swap Launcher Dev</string>
	<key>CFBundleIdentifier</key>
	<string>com.my-wang.llama-swap-launcher.dev</string>
	<key>LSUIElement</key>
	<true/>
</dict>
</plist>
`), 0o600)
	if got := plistBundleID(path); got != "com.my-wang.llama-swap-launcher.dev" {
		t.Errorf("got %q", got)
	}
	if got := plistBundleID(filepath.Join(t.TempDir(), "missing.plist")); got != "" {
		t.Errorf("missing file: got %q", got)
	}
}

func TestUsageErrors(t *testing.T) {
	for _, args := range [][]string{
		nil,
		{"frobnicate"},
		{"unload"},
		{"unload", "m", "-all"},
		{"status", "extra"},
		{"start", "-bogus"},
	} {
		var out, errOut bytes.Buffer
		if got := run(args, &out, &errOut); got != exitUsage {
			t.Errorf("llsl %q: exit %d, want %d", args, got, exitUsage)
		}
	}
}

// An app from before the socket was renamed, still running after an
// upgrade, is reported as such rather than as not running.
func TestOldAppRunning(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "lsl") // t.TempDir() can be too long for a socket path
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	p := paths.Paths{SupportDir: dir}
	newCLI := func(out, errOut *bytes.Buffer) *cli {
		return &cli{id: "test", paths: p, stdout: out, stderr: errOut}
	}

	// Nothing running at all: the usual "not running".
	var out, errOut bytes.Buffer
	if got := newCLI(&out, &errOut).simple("status", nil); got != exitNoApp || !strings.Contains(out.String(), "not running") {
		t.Errorf("no app: exit %d, stdout %q", got, out.String())
	}

	l, err := control.Listen(p.OldSocket())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	go control.Serve(l, func(ctx context.Context, req control.Request) control.Response {
		return control.Response{OK: true, Status: &control.Status{State: "ready"}}
	})

	for _, cmd := range []string{"status", "stop"} {
		out.Reset()
		errOut.Reset()
		got := newCLI(&out, &errOut).simple(cmd, nil)
		if got != exitNoApp || !strings.Contains(errOut.String(), "older version") {
			t.Errorf("%s with an old app: exit %d, stderr %q", cmd, got, errOut.String())
		}
	}
}

func TestWarnVersion(t *testing.T) {
	defer func(v string) { version = v }(version)
	for _, tt := range []struct {
		llsl, app string
		warn      bool
	}{
		{"0.1.3", "0.1.3", false},
		{"0.1.4", "0.1.3", true},
		{"dev", "0.1.3", false}, // built without the Makefile
		{"0.1.3", "", false},
	} {
		version = tt.llsl
		var errOut bytes.Buffer
		(&cli{stderr: &errOut}).warnVersion(&control.Status{AppVersion: tt.app})
		if got := errOut.Len() > 0; got != tt.warn {
			t.Errorf("llsl %q, app %q: warned %v, want %v (%q)", tt.llsl, tt.app, got, tt.warn, errOut.String())
		}
	}
}

func TestForceFlag(t *testing.T) {
	for _, args := range [][]string{{"status", "-force"}, {"start", "-force"}} {
		var out, errOut bytes.Buffer
		if got := run(args, &out, &errOut); got != exitUsage {
			t.Errorf("llsl %q: exit %d, want %d (-force is only for restart, stop and unload)", args, got, exitUsage)
		}
	}
	if got := exitCode(control.Response{Code: control.CodeBusy}, false); got != exitBusy || exitBusy != 7 {
		t.Errorf("busy: exit %d, want 7", got)
	}
}

// Text from elsewhere can't redraw the terminal or forge a line.
func TestPrintStatusEscapes(t *testing.T) {
	var out bytes.Buffer
	printStatus(&out, &control.Status{
		State:   "ready",
		Message: "fine\nllama-swap: ready (forged)",
		Peers:   []control.Peer{{ID: "p", Error: "boom \x1b[2J\x1b[H cleared"}},
	})
	got := out.String()
	if strings.Contains(got, "\x1b") || strings.Contains(got, "\nllama-swap: ready (forged)") {
		t.Errorf("control characters got through:\n%q", got)
	}
	if !strings.Contains(got, `\x1b[2J`) || !strings.Contains(got, `fine\x0allama-swap`) {
		t.Errorf("not shown escaped:\n%q", got)
	}
}

// check-update asks the app and exits 9 when a newer release is out, 0 when
// not, 1 when the check failed, and 3 with no app (TODO item 64).
func TestCheckUpdate(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "lsl") // t.TempDir() can be too long for a socket path
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	p := paths.Paths{SupportDir: dir}
	run := func(args ...string) (int, string, string) {
		var out, errOut bytes.Buffer
		code := (&cli{id: "test", paths: p, stdout: &out, stderr: &errOut}).checkUpdate(args)
		return code, out.String(), errOut.String()
	}
	if code, _, _ := run(); code != exitNoApp {
		t.Errorf("no app: exit %d", code)
	}

	l, err := control.Listen(p.Socket())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	var answer control.Response
	go control.Serve(l, func(ctx context.Context, req control.Request) control.Response {
		if req.Cmd != "check-update" {
			return control.Failure(control.CodeUsage, errors.New("not check-update"))
		}
		return answer
	})

	answer = control.Response{OK: true, Update: &control.Update{Current: "0.3.3", Latest: "0.3.4", Available: true,
		URL: "https://github.com/alron/llama-swap-mac-launcher/releases/tag/v0.3.4", Homebrew: true}}
	code, out, _ := run()
	if code != exitUpdate || !strings.Contains(out, "0.3.4 is available (this is 0.3.3)") || !strings.Contains(out, "brew upgrade") {
		t.Errorf("newer: exit %d, %q", code, out)
	}
	code, out, _ = run("-json")
	if code != exitUpdate || !strings.Contains(out, `"available": true`) {
		t.Errorf("newer, JSON: exit %d, %q", code, out)
	}

	answer = control.Response{OK: true, Update: &control.Update{Current: "0.3.4", Latest: "0.3.4"}}
	if code, out, _ := run(); code != exitOK || !strings.Contains(out, "0.3.4 is the newest release") || strings.Contains(out, "brew") {
		t.Errorf("up to date: exit %d, %q", code, out)
	}

	answer = control.Failure(control.CodeError, errors.New("couldn't check for updates: GitHub answered HTTP 403"))
	if code, _, errOut := run(); code != exitFailed || !strings.Contains(errOut, "403") {
		t.Errorf("failed: exit %d, %q", code, errOut)
	}
	if code, _, _ := run("extra"); code != exitUsage {
		t.Errorf("an argument: exit %d", code)
	}
}

// status's line for the GPU's memory limit (TODO item 51).
func TestGPUMemoryLine(t *testing.T) {
	const gb = 1024
	for _, tt := range []struct {
		g    control.GPUMemory
		want string
	}{
		{control.GPUMemory{TotalMB: 96 * gb, DefaultMB: 79626, Helper: "not registered"}, "macOS's default, about 78 GB of 96 GB"},
		{control.GPUMemory{TotalMB: 96 * gb, LimitMB: 84 * gb, StoredMB: 84 * gb, Helper: "enabled", Running: true, SetByApp: true}, "84 GB of 96 GB, set by the app"},
		{control.GPUMemory{TotalMB: 96 * gb, LimitMB: 80 * gb, StoredMB: 80 * gb, Helper: "enabled", SetByApp: true},
			"80 GB of 96 GB, set by the app; the app's helper is allowed but not running (Save in Settings starts it again)"},
		{control.GPUMemory{TotalMB: 256 * gb, LimitMB: 224 * gb, Helper: "not registered"}, "224 GB of 256 GB, not set by the app"},
		{control.GPUMemory{TotalMB: 96 * gb, DefaultMB: 79626, Helper: "requires approval", PendingMB: 84 * gb},
			"macOS's default, about 78 GB of 96 GB; 84 GB once the app's helper is allowed in System Settings (if it's allowed and nothing happens, Save in Settings again)"},
		{control.GPUMemory{TotalMB: 96 * gb, LimitMB: 70 * gb, StoredMB: 84 * gb, Helper: "enabled", Running: true},
			"70 GB of 96 GB, not set by the app; the app sets 84 GB at the next startup"},
	} {
		if got := gpuMemory(&tt.g); got != tt.want {
			t.Errorf("%+v:\n got %q\nwant %q", tt.g, got, tt.want)
		}
	}
}
