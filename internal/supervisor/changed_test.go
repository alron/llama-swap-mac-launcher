package supervisor

import (
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A server started through a symlink, which is then repointed to a newer
// copy (as "brew upgrade" does), reads as changed; before that, it doesn't.
func TestServerChanged(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(self)
	dir := t.TempDir()
	for _, v := range []string{"1", "2"} {
		os.MkdirAll(filepath.Join(dir, "Cellar", v), 0o700)
	}
	os.WriteFile(filepath.Join(dir, "Cellar", "1", "server"), b, 0o700)
	link := filepath.Join(dir, "server")
	os.Symlink(filepath.Join("Cellar", "1", "server"), link)

	l, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := l.Addr().String()
	l.Close()
	cmd := exec.Command(link)
	cmd.Env = append(os.Environ(), listenEnv+"="+addr)
	stdin, _ := cmd.StdinPipe()
	out, _ := cmd.StdoutPipe()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { stdin.Close(); cmd.Wait() }()
	buf := make([]byte, 16)
	out.Read(buf) // "ready"
	_, port, _ := net.SplitHostPort(addr)

	changed, program, err := ServerChanged(port, dir)
	if err != nil || changed || program != link {
		t.Fatalf("before the upgrade: changed %v, program %q, err %v", changed, program, err)
	}

	time.Sleep(10 * time.Millisecond) // a ctime after the start
	os.WriteFile(filepath.Join(dir, "Cellar", "2", "server"), b, 0o700)
	old := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC) // as a bottle's files can be
	os.Chtimes(filepath.Join(dir, "Cellar", "2", "server"), old, old)
	os.Remove(link)
	os.Symlink(filepath.Join("Cellar", "2", "server"), link)
	os.RemoveAll(filepath.Join(dir, "Cellar", "1"))

	changed, _, err = ServerChanged(port, dir)
	if err != nil || !changed {
		t.Errorf("after the upgrade: changed %v, err %v", changed, err)
	}
}

func TestServerChangedNothingListening(t *testing.T) {
	if _, _, err := ServerChanged("1", "/"); err == nil || !strings.Contains(err.Error(), "nothing is listening") {
		t.Errorf("got %v", err)
	}
}
