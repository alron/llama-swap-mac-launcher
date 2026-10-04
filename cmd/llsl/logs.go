package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/alron/llama-swap-mac-launcher/internal/control"
	"github.com/alron/llama-swap-mac-launcher/internal/prefs"
)

// logs prints the end of a log, and with -f follows it: where llama-swap's
// output goes, or with -launcher the launcher log. It finds them through
// the app's preferences and reads them directly, so it works whether or
// not the app is running. When llama-swap's output isn't saved, it asks
// the app for the last lines it keeps in memory instead.
func (c *cli) logs(args []string) int {
	fs := c.flags("logs")
	n := fs.Int("n", 50, "")
	follow := fs.Bool("f", false, "")
	launcherOnly := fs.Bool("launcher", false, "")
	if fs.Parse(args) != nil || fs.NArg() > 0 {
		return exitUsage
	}
	p, err := prefs.Load(c.paths.Prefs())
	if err != nil {
		fmt.Fprintf(c.stderr, "llsl: %v; using the default log locations\n", err)
	}
	launcher, own := p.LogPaths(c.paths.LogDir)
	path := launcher
	switch {
	case *launcherOnly:
	case p.LogMode() == prefs.LogToFile:
		path = own
	case p.LogMode() == prefs.LogOff:
		return c.memoryOutput(*n, *follow)
	}
	b, err := os.ReadFile(path) // #nosec G304 -- one of the app's logs, from its preferences
	if err != nil && !(*follow && os.IsNotExist(err)) {
		fmt.Fprintln(c.stderr, "llsl:", err)
		return exitFailed
	}
	_, _ = c.stdout.Write(lastLines(b, *n)) // nothing useful to do if stdout is gone
	if !*follow {
		return exitOK
	}
	followFile(path, int64(len(b)), c.stdout)
	return exitOK
}

// memoryOutput prints the last lines of llama-swap's output that the app
// keeps in memory, for when the preferences don't save them.
func (c *cli) memoryOutput(n int, follow bool) int {
	if follow {
		fmt.Fprintln(c.stderr, "llsl: llama-swap's output isn't saved (Settings… → Logs), so there's no log to follow. llsl logs -launcher -f follows the launcher log")
		return exitUsage
	}
	resp, err := c.call(control.Request{Cmd: "output", Lines: n}, 10*time.Second)
	if err != nil {
		fmt.Fprintln(c.stderr, "llsl: llama-swap's output isn't saved, and the app, which keeps its last lines, can't be reached:", err)
		return exitNoApp
	}
	fmt.Fprintf(c.stderr, "llsl: llama-swap's output isn't saved; these are the last %d lines the app kept in memory\n", len(resp.Output))
	for _, line := range resp.Output {
		fmt.Fprintln(c.stdout, line)
	}
	return exitOK
}

// lastLines returns the last n lines of b.
func lastLines(b []byte, n int) []byte {
	if n <= 0 || len(b) == 0 {
		return nil
	}
	i := len(b)
	if b[i-1] == '\n' {
		i-- // a trailing newline doesn't start another line
	}
	for ; n > 0; n-- {
		j := bytes.LastIndexByte(b[:i], '\n')
		if j < 0 {
			return b
		}
		i = j
	}
	return b[i+1:]
}

// followFile copies what's appended to path from offset on, forever. When
// the log is rotated (replaced by a new file, or truncated), it starts on
// the new file from the beginning.
func followFile(path string, offset int64, w io.Writer) {
	var f *os.File
	for {
		if f == nil {
			var err error
			if f, err = os.Open(path); err != nil { // #nosec G304 -- the app's own log file
				time.Sleep(time.Second)
				continue
			}
			// Without the seek, the copy below would reprint the whole log.
			if _, err := f.Seek(offset, io.SeekStart); err != nil {
				_ = f.Close() // read-only, so nothing to lose
				f = nil
				time.Sleep(time.Second)
				continue
			}
		}
		n, _ := io.Copy(w, f)
		offset += n
		time.Sleep(250 * time.Millisecond)

		cur, err1 := os.Stat(path)
		open, err2 := f.Stat()
		if err1 != nil || err2 != nil || !os.SameFile(cur, open) || cur.Size() < offset {
			if err2 == nil {
				_, _ = io.Copy(w, f) // the rest of the old file first
			}
			_ = f.Close() // read-only, so nothing to lose
			f, offset = nil, 0
		}
	}
}
