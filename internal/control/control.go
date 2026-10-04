// Package control is the app's control socket: the protocol, the server the
// app runs, and the client llsl uses.
//
// It's a Unix domain socket in the app's support directory. Unix sockets
// aren't IP traffic, so Local Network privacy doesn't apply to them. Each
// connection carries one request and one response, each a line of JSON.
//
// This package must stay free of cgo and AppKit: llsl imports it.
package control

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

type Request struct {
	Cmd string `json:"cmd"` // status, start, stop, restart, unload, output
	// Wait makes start and restart return only once llama-swap is ready,
	// or has failed.
	Wait bool `json:"wait,omitempty"`
	// Load makes start and restart also load this model, and wait until
	// it's ready or has failed. It implies Wait.
	Load string `json:"load,omitempty"`
	// Model and All pick what unload unloads.
	Model string `json:"model,omitempty"`
	All   bool   `json:"all,omitempty"`
	// TimeoutMs bounds Wait and Load.
	TimeoutMs int64 `json:"timeoutMs,omitempty"`
	// Force makes stop, restart and unload go ahead even while llama-swap
	// is serving requests they would cut short.
	Force bool `json:"force,omitempty"`
	// Lines is how many of llama-swap's last output lines output returns.
	Lines int `json:"lines,omitempty"`
}

// Codes say why a request failed. llsl turns them into exit codes.
const (
	CodeUsage         = "usage"          // a malformed request
	CodeInvalidConfig = "invalid_config" // the config didn't validate; llama-swap was left as it was
	CodeStartFailed   = "start_failed"   // llama-swap couldn't start, or exited
	CodeLoadFailed    = "load_failed"    // the model didn't load
	CodeTimeout       = "timeout"
	CodeBusy          = "busy"  // requests were in flight, so nothing was done; Force overrides
	CodeError         = "error" // anything else
)

type Response struct {
	OK     bool    `json:"ok"`
	Code   string  `json:"code,omitempty"`
	Error  string  `json:"error,omitempty"`
	Status *Status `json:"status,omitempty"`
	// Output is the end of llama-swap's output, from the app's memory: on
	// a failure that llama-swap's own words may explain, and from output.
	Output []string `json:"output,omitempty"`
}

type Status struct {
	AppPid     int    `json:"appPid"`
	AppVersion string `json:"appVersion"`
	BundleID   string `json:"bundleID"`
	// State is llama-swap's: stopped, starting, ready, not responding,
	// stopping, restarting (after a crash) or failed.
	State   string `json:"state"`
	Pid     int    `json:"pid,omitempty"`
	Listen  string `json:"listen,omitempty"`
	Version string `json:"version,omitempty"` // the running llama-swap's, such as "v262 (079c35a)"
	Message string `json:"message,omitempty"`
	// Warning says when llama-swap can be used without an API key: the app
	// has keys its config doesn't list, or it listens beyond this Mac with
	// none.
	Warning string `json:"warning,omitempty"`
	Binary  string `json:"binary,omitempty"`
	Config  string `json:"config,omitempty"`
	LogFile string `json:"logFile"` // where llama-swap's output goes; "" when it isn't saved
	// LauncherLog is the app's own log, which also has llama-swap's
	// output unless the preferences give it its own file.
	LauncherLog string  `json:"launcherLog"`
	Models      []Model `json:"models"` // loaded local models
	Peers       []Peer  `json:"peers,omitempty"`
	// LastGPUFault is the most recent model whose GPU backend failed since
	// the app started, kept after the model is unloaded and the fault gone.
	LastGPUFault *Fault `json:"lastGpuFault,omitempty"`
}

// Fault records a model whose GPU backend failed.
type Fault struct {
	Model        string    `json:"model"`
	At           time.Time `json:"at"`
	AutoUnloaded bool      `json:"autoUnloaded,omitempty"` // by the unloadOnGpuFault preference
}

// Peer is another llama-swap whose models this one serves, and whether it
// answered when asked through llama-swap (status requests only).
type Peer struct {
	ID    string `json:"id"`
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"` // why not, as llama-swap reported it
}

type Model struct {
	ID    string `json:"id"`
	State string `json:"state"`
	// Faulted means its GPU backend has failed; see internal/health.
	Faulted bool `json:"faulted,omitempty"`
	// LastTokensPerSecond is its average generation speed in its latest
	// request since it loaded (not a live reading); omitted until it has
	// generated something.
	LastTokensPerSecond float64 `json:"lastTokensPerSecond,omitempty"`
	// Updated means its server program has changed on disk since it
	// loaded, so it runs the old build until unloaded (only with the
	// markUpdatedModels preference).
	Updated bool `json:"updated,omitempty"`
}

// Failure builds a failed response.
func Failure(code string, err error) Response {
	return Response{Code: code, Error: err.Error()}
}

// ErrAlreadyRunning means another instance of the app holds the socket.
var ErrAlreadyRunning = errors.New("another instance of the app is already running")

// maxSocketPath is the longest path a Unix socket can have on macOS:
// sockaddr_un's 104-byte sun_path, less the terminating NUL.
const maxSocketPath = 103

// Listen opens the socket at path. Holding it is also the app's
// single-instance lock: it fails with ErrAlreadyRunning if another
// instance answers there, and replaces a socket left by one that crashed.
// The directory should be readable only by the user.
func Listen(path string) (*net.UnixListener, error) {
	if len(path) > maxSocketPath {
		return nil, fmt.Errorf("the control socket path is %d bytes, more than macOS allows (%d): %s",
			len(path), maxSocketPath, path)
	}
	if conn, err := net.DialTimeout("unix", path, time.Second); err == nil {
		_ = conn.Close() // only probing whether another instance answers
		return nil, ErrAlreadyRunning
	} else if errors.Is(err, syscall.ECONNREFUSED) {
		_ = os.Remove(path) // left by an instance that crashed
	}
	l, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if errors.Is(err, syscall.EADDRINUSE) {
		// Two copies opened at once, and the other bound first.
		if conn, derr := net.DialTimeout("unix", path, time.Second); derr == nil {
			_ = conn.Close() // only probing whether another instance answers
			return nil, ErrAlreadyRunning
		}
	}
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = l.Close() // already failing; the Chmod error is the one to report
		return nil, err
	}
	return l, nil
}

// Handler answers requests. ctx ends when the client hangs up.
type Handler func(ctx context.Context, req Request) Response

// Serve answers connections on l until it's closed.
func Serve(l *net.UnixListener, h Handler) {
	for {
		conn, err := l.AcceptUnix()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			continue
		}
		go serveConn(conn, h)
	}
}

func serveConn(conn *net.UnixConn, h Handler) {
	defer conn.Close()
	if !sameUser(conn) {
		return
	}
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second)) // fails only on a closed connection, which the read then reports
	// A request is one short line of JSON; a longer one is refused.
	r := bufio.NewReader(io.LimitReader(conn, maxRequest))
	line, err := r.ReadBytes('\n')
	if err != nil {
		return
	}
	var req Request
	var resp Response
	if err := json.Unmarshal(line, &req); err != nil {
		resp = Failure(CodeUsage, fmt.Errorf("bad request: %w", err))
	} else {
		// The client sends nothing more, so a read that returns means it
		// hung up: stop waiting on its behalf.
		ctx, cancel := context.WithCancel(context.Background())
		_ = conn.SetReadDeadline(time.Time{}) // fails only on a closed connection
		go func() {
			_, _ = r.ReadByte() // any result, data or error, means the client is done
			cancel()
		}()
		resp = h(ctx, req)
		cancel()
	}
	b, _ := json.Marshal(resp)
	_, _ = conn.Write(append(b, '\n')) // if the client has gone, there's no one to tell
}

// sameUser reports whether the peer runs as this process's user. The
// socket's permissions already say so; this is a second check.
func sameUser(conn *net.UnixConn) bool {
	raw, err := conn.SyscallConn()
	if err != nil {
		return false
	}
	ok := false
	_ = raw.Control(func(fd uintptr) { // if it fails, ok stays false and the peer is refused
		cred, err := unix.GetsockoptXucred(int(fd), unix.SOL_LOCAL, unix.LOCAL_PEERCRED)
		ok = err == nil && int(cred.Uid) == os.Getuid()
	})
	return ok
}

// maxRequest caps a request's size: one line of JSON, which is short.
const maxRequest = 64 << 10

// ErrNotRunning means nothing answers at the socket: the app isn't running.
var ErrNotRunning = errors.New("the app isn't running")

// Call sends req to the app at path and returns its response, waiting at
// most timeout for it.
func Call(path string, req Request, timeout time.Duration) (Response, error) {
	conn, err := net.DialTimeout("unix", path, 2*time.Second)
	if err != nil {
		if errors.Is(err, syscall.ENOENT) || errors.Is(err, syscall.ECONNREFUSED) {
			return Response{}, ErrNotRunning
		}
		return Response{}, err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(timeout)) // fails only on a closed connection, which the write then reports
	b, err := json.Marshal(req)
	if err != nil {
		return Response{}, err
	}
	if _, err := conn.Write(append(b, '\n')); err != nil {
		return Response{}, err
	}
	line, err := bufio.NewReader(conn).ReadBytes('\n')
	if err != nil {
		return Response{}, fmt.Errorf("no answer from the app: %w", err)
	}
	var resp Response
	if err := json.Unmarshal(line, &resp); err != nil {
		return Response{}, fmt.Errorf("bad answer from the app: %w", err)
	}
	return resp, nil
}
