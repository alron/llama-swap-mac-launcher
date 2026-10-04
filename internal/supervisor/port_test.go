package supervisor

import (
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// listenEnv makes the test binary, run under another name, a process that
// listens on the address it gives, prints "ready", and waits.
const listenEnv = "SUPERVISOR_TEST_LISTEN"

func TestMain(m *testing.M) {
	if addr := os.Getenv(listenEnv); addr != "" {
		l, err := net.Listen("tcp", addr)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		defer l.Close()
		fmt.Println("ready")
		_, _ = io.Copy(io.Discard, os.Stdin) // until the test closes stdin
		return
	}
	os.Exit(m.Run())
}

// listenAs runs a copy of the test binary named name, listening on a free
// local port, and returns that address and the process.
func listenAs(t *testing.T, name string) (string, *exec.Cmd) {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(self)
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(bin, b, 0o700); err != nil {
		t.Fatal(err)
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	l.Close()

	cmd := exec.Command(bin)
	cmd.Env = append(os.Environ(), listenEnv+"="+addr)
	stdin, _ := cmd.StdinPipe()
	out, _ := cmd.StdoutPipe()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		stdin.Close()
		cmd.Wait()
	})
	buf := make([]byte, 64)
	n, _ := out.Read(buf)
	if !strings.Contains(string(buf[:n]), "ready") {
		t.Fatalf("%s didn't start listening: %q", name, buf[:n])
	}
	return addr, cmd
}

func TestPortTakenNamesHolder(t *testing.T) {
	addr, cmd := listenAs(t, "llama-swap")
	err := checkPortFree(addr)
	if err == nil {
		t.Fatal("no error although the port is taken")
	}
	// Its parent is this test, not the app.
	want := fmt.Sprintf("llama-swap (pid %d), not started by this app, is already listening on %s. Stop it (kill %d)",
		cmd.Process.Pid, addr, cmd.Process.Pid)
	if !strings.Contains(err.Error(), want) {
		t.Errorf("got %q\nwant it to contain %q", err, want)
	}

	addr, cmd = listenAs(t, "some-server")
	err = checkPortFree(addr)
	want = fmt.Sprintf("some-server (pid %d) is already listening", cmd.Process.Pid)
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("got %v\nwant it to contain %q", err, want)
	}
}

func TestPortTakenUnknownHolder(t *testing.T) {
	err := portTaken("127.0.0.1:8080", nil)
	if err == nil || !strings.Contains(err.Error(), "something is already listening on 127.0.0.1:8080") {
		t.Errorf("got %v", err)
	}
}
