// Package logfile is a size-rotated log file. When a write would take the
// file past its size limit, the file becomes <name>.1, the old .1 becomes
// .2, and so on, keeping a fixed number of old files.
package logfile

import (
	"fmt"
	"os"
	"sync"
	"syscall"
)

type Rotating struct {
	mu     sync.Mutex
	path   string
	max    int64
	keep   int
	f      *os.File
	size   int64
	onOpen func(*os.File)
}

// Open opens (appending) or creates the log at path. It rotates once the
// file would exceed maxSize bytes, keeping keep old files (at least 1).
func Open(path string, maxSize int64, keep int) (*Rotating, error) {
	r := &Rotating{path: path, max: maxSize, keep: max(keep, 1)}
	if err := r.open(); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *Rotating) open() error {
	// O_NOFOLLOW: in a folder others can write to (/tmp, say), a symlink
	// waiting at the log's name would otherwise take the app's writes
	// somewhere else.
	f, err := os.OpenFile(r.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return err
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close() // the Stat error is the one to report
		return err
	}
	r.f, r.size = f, info.Size()
	if r.onOpen != nil {
		r.onOpen(f)
	}
	return nil
}

// OnOpen calls f with the log's file now, and again each time rotation
// opens a new one: the app points its stderr at the file, so a crash's
// stack trace lands in the current log.
func (r *Rotating) OnOpen(f func(*os.File)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.onOpen = f
	f(r.f)
}

func (r *Rotating) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.size > 0 && r.size+int64(len(p)) > r.max {
		if err := r.rotate(); err != nil {
			return 0, err
		}
	}
	n, err := r.f.Write(p)
	r.size += int64(n)
	return n, err
}

func (r *Rotating) rotate() error {
	if err := r.f.Close(); err != nil {
		return err
	}
	for i := r.keep - 1; i >= 1; i-- {
		_ = os.Rename(r.name(i), r.name(i+1)) // a missing file just means there's nothing to shift yet
	}
	if err := os.Rename(r.path, r.name(1)); err != nil {
		return err
	}
	return r.open()
}

func (r *Rotating) name(i int) string { return fmt.Sprintf("%s.%d", r.path, i) }

// Path is the current log file.
func (r *Rotating) Path() string { return r.path }

func (r *Rotating) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.f.Close()
}
