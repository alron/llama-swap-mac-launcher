package logfile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRotateKeepsNewestFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.log")
	r, err := Open(path, 10, 2)
	if err != nil {
		t.Fatal(err)
	}
	// Each line is 6 bytes, so every write after the first rotates.
	for _, line := range []string{"line1\n", "line2\n", "line3\n", "line4\n"} {
		if _, err := r.Write([]byte(line)); err != nil {
			t.Fatal(err)
		}
	}
	r.Close()

	want := map[string]string{
		path:        "line4\n",
		path + ".1": "line3\n",
		path + ".2": "line2\n",
	}
	for p, content := range want {
		got, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != content {
			t.Errorf("%s = %q, want %q", filepath.Base(p), got, content)
		}
	}
	if _, err := os.Stat(path + ".3"); !os.IsNotExist(err) {
		t.Errorf(".3 exists, but only 2 old files should be kept")
	}
}

func TestOpenAppends(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.log")
	os.WriteFile(path, []byte("old\n"), 0o600)
	r, err := Open(path, 1000, 1)
	if err != nil {
		t.Fatal(err)
	}
	r.Write([]byte("new\n"))
	r.Close()
	if got, _ := os.ReadFile(path); string(got) != "old\nnew\n" {
		t.Errorf("got %q", got)
	}
}

// A symlink waiting at the log's name isn't followed.
func TestOpenRefusesSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "elsewhere")
	os.WriteFile(target, nil, 0o600)
	link := filepath.Join(dir, "x.log")
	os.Symlink(target, link)
	if r, err := Open(link, 1<<20, 2); err == nil {
		r.Close()
		t.Error("opened a log through a symlink")
	}
}
