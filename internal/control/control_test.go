package control

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// socketPath returns a short socket path: t.TempDir() on macOS can be longer
// than a Unix socket path may be.
func socketPath(t *testing.T) string {
	dir, err := os.MkdirTemp("/tmp", "lsl")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return filepath.Join(dir, "control.sock")
}

func serve(t *testing.T, path string, h Handler) *net.UnixListener {
	l, err := Listen(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	go Serve(l, h)
	return l
}

func TestRoundTrip(t *testing.T) {
	path := socketPath(t)
	serve(t, path, func(ctx context.Context, req Request) Response {
		return Response{OK: true, Status: &Status{State: req.Cmd + " " + req.Load}}
	})
	resp, err := Call(path, Request{Cmd: "start", Load: "m"}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !resp.OK || resp.Status.State != "start m" {
		t.Errorf("got %+v", resp)
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o600 {
		t.Errorf("socket mode %v, want 0600", info.Mode().Perm())
	}
}

func TestSecondInstanceRefused(t *testing.T) {
	path := socketPath(t)
	serve(t, path, func(context.Context, Request) Response { return Response{OK: true} })
	if _, err := Listen(path); !errors.Is(err, ErrAlreadyRunning) {
		t.Errorf("second Listen: got %v, want ErrAlreadyRunning", err)
	}
}

func TestStaleSocketReplaced(t *testing.T) {
	path := socketPath(t)
	l, err := Listen(path)
	if err != nil {
		t.Fatal(err)
	}
	l.SetUnlinkOnClose(false) // like a crash: the socket file stays behind
	l.Close()
	if _, err := os.Stat(path); err != nil {
		t.Fatal("stale socket file should still exist")
	}
	serve(t, path, func(context.Context, Request) Response { return Response{OK: true} })
	if _, err := Call(path, Request{Cmd: "status"}, time.Second); err != nil {
		t.Errorf("after replacing the stale socket: %v", err)
	}
}

func TestCallWhenNotRunning(t *testing.T) {
	if _, err := Call(socketPath(t), Request{Cmd: "status"}, time.Second); !errors.Is(err, ErrNotRunning) {
		t.Errorf("got %v, want ErrNotRunning", err)
	}
}

func TestPathTooLong(t *testing.T) {
	if _, err := Listen("/tmp/" + strings.Repeat("x", 100) + ".sock"); err == nil {
		t.Error("no error for a path longer than macOS allows")
	}
}

func TestHandlerContextEndsWhenClientHangsUp(t *testing.T) {
	path := socketPath(t)
	cancelled := make(chan struct{})
	serve(t, path, func(ctx context.Context, req Request) Response {
		<-ctx.Done()
		close(cancelled)
		return Response{}
	})
	conn, err := net.Dial("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	conn.Write([]byte(`{"cmd":"restart","wait":true}` + "\n"))
	time.Sleep(100 * time.Millisecond)
	conn.Close()
	select {
	case <-cancelled:
	case <-time.After(2 * time.Second):
		t.Error("handler kept waiting after the client hung up")
	}
}
