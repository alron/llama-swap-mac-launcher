package logfile

import (
	"bytes"
	"io"
	"sync"
)

// maxPartial is how much of an unfinished line DropLines holds back before
// passing it on anyway.
const maxPartial = 64 << 10

// DropLines returns a writer that passes whole lines on to w, except those
// containing drop. It holds an unfinished line back until its newline
// arrives.
func DropLines(w io.Writer, drop string) io.Writer {
	return &lineFilter{w: w, drop: []byte(drop)}
}

type lineFilter struct {
	mu   sync.Mutex
	w    io.Writer
	drop []byte
	buf  []byte
}

func (f *lineFilter) Write(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.buf = append(f.buf, p...)
	var out []byte
	rest := f.buf
	for {
		i := bytes.IndexByte(rest, '\n')
		if i < 0 {
			break
		}
		if line := rest[:i+1]; !bytes.Contains(line, f.drop) {
			out = append(out, line...)
		}
		rest = rest[i+1:]
	}
	if len(rest) > maxPartial {
		out = append(out, rest...)
		rest = nil
	}
	f.buf = append([]byte(nil), rest...)
	if len(out) > 0 {
		if _, err := f.w.Write(out); err != nil {
			return 0, err
		}
	}
	return len(p), nil
}
