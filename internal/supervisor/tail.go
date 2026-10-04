package supervisor

import (
	"bytes"
	"strings"
	"sync"
)

// outputLines is how many lines of llama-swap's output the supervisor keeps
// in memory, for when it fails.
const outputLines = 100

// maxLineLen caps each kept line, so one huge line can't hold much memory.
const maxLineLen = 1000

// tail keeps the last lines written to it. It's an io.Writer, written by
// the goroutine copying llama-swap's output and read by others.
type tail struct {
	mu      sync.Mutex
	lines   []string // oldest first, at most outputLines
	partial []byte   // the start of a line not yet ended by a newline
}

func (t *tail) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	n := len(p)
	for len(p) > 0 {
		i := bytes.IndexByte(p, '\n')
		if i < 0 {
			t.partial = append(t.partial, p...)
			if len(t.partial) > maxLineLen {
				t.partial = t.partial[:maxLineLen]
			}
			break
		}
		t.partial = append(t.partial, p[:i]...)
		t.add(string(t.partial))
		t.partial = t.partial[:0]
		p = p[i+1:]
	}
	return n, nil
}

func (t *tail) add(line string) {
	line = strings.TrimSuffix(line, "\r")
	if len(line) > maxLineLen {
		line = line[:maxLineLen]
	}
	t.lines = append(t.lines, line)
	if len(t.lines) > outputLines {
		t.lines = t.lines[len(t.lines)-outputLines:]
	}
}

// last returns up to n of the most recent lines, oldest first, including
// one still being written.
func (t *tail) last(n int) []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	all := t.lines
	if len(t.partial) > 0 {
		all = append(all[:len(all):len(all)], string(t.partial))
	}
	if len(all) > n {
		all = all[len(all)-n:]
	}
	return append([]string(nil), all...)
}

func (t *tail) reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.lines, t.partial = nil, t.partial[:0]
}
