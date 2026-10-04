package logfile

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type failing struct{}

func (failing) Write([]byte) (int, error) { return 0, errors.New("disk full") }

func TestSwitch(t *testing.T) {
	var a, b bytes.Buffer
	s := NewSwitch(&a)
	s.Write([]byte("one\n"))
	s.Set(&b)
	s.Write([]byte("two\n"))
	if a.String() != "one\n" || b.String() != "two\n" {
		t.Errorf("a %q, b %q", a.String(), b.String())
	}
	s.Set(failing{})
	if n, err := s.Write([]byte("three\n")); n != 6 || err != nil {
		t.Errorf("a failed write returned %d, %v; want it swallowed", n, err)
	}
	if s.Err() == nil {
		t.Error("the failure wasn't kept")
	}
}

func TestOnOpenFollowsRotation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.log")
	r, err := Open(path, 10, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	var opened []*os.File
	r.OnOpen(func(f *os.File) { opened = append(opened, f) })
	r.Write([]byte("0123456789"))
	r.Write([]byte("more")) // rotates
	if len(opened) != 2 || opened[0] == opened[1] {
		t.Errorf("OnOpen saw %d files, want the first and the rotated one", len(opened))
	}
}
