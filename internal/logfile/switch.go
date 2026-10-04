package logfile

import (
	"io"
	"sync"
)

// Switch is a writer whose destination can change while it's in use, so a
// log can move without whoever writes to it knowing. Each write goes
// wholly to one destination.
//
// It never fails. llama-swap's output is copied into a pipe that must keep
// draining: an error would stop the copying, and llama-swap would block
// once the pipe filled. So a failed write (a full disk, say) is dropped,
// and the first such error kept for Err.
type Switch struct {
	mu  sync.Mutex
	w   io.Writer
	err error
}

func NewSwitch(w io.Writer) *Switch { return &Switch{w: w} }

// Set sends later writes to w.
func (s *Switch) Set(w io.Writer) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.w = w
}

func (s *Switch) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.w.Write(p); err != nil && s.err == nil {
		s.err = err
	}
	return len(p), nil
}

// Err returns the first write that failed, if any.
func (s *Switch) Err() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}
