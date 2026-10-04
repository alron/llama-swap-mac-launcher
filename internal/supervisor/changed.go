package supervisor

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// ServerChanged reports whether the program listening on port (a model's
// server) has changed on disk since that process started, as after a
// "brew upgrade", and returns the program's path as it was started. A
// relative path resolves from dir, llama-swap's working folder.
//
// It follows the path the process was started as (a symlink such as
// /opt/homebrew/bin/llama-server, into a versioned folder) to the file
// there now, and compares that file's change time (ctime) with the
// process's start. ctime rather than mtime: files unpacked from a Homebrew
// bottle can keep an old mtime, but unpacking sets their ctime.
func ServerChanged(port, dir string) (changed bool, program string, err error) {
	holders := listeners(port)
	if len(holders) == 0 {
		return false, "", errors.New("nothing is listening on port " + port)
	}
	p := holders[0]
	program, err = startedAs(p.Pid)
	if err != nil {
		return false, "", err
	}
	path := program
	if !filepath.IsAbs(path) {
		path = filepath.Join(dir, path)
	}
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		// The file it was started from is gone, and nothing replaced it.
		return true, program, nil
	}
	info, err := os.Stat(real)
	if err != nil {
		return false, program, err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return false, program, errors.New("no file times for " + real)
	}
	ctime := time.Unix(st.Ctimespec.Sec, st.Ctimespec.Nsec)
	started := time.UnixMicro(p.Start)
	return ctime.After(started), program, nil
}

// startedAs returns the path the process was started as: kern.procargs2
// holds argc, then that path, NUL-terminated, then the arguments.
func startedAs(pid int) (string, error) {
	b, err := unix.SysctlRaw("kern.procargs2", pid)
	if err != nil {
		return "", err
	}
	if len(b) < 5 {
		return "", errors.New("no arguments recorded for the process")
	}
	path, _, _ := bytes.Cut(b[4:], []byte{0})
	return string(path), nil
}
