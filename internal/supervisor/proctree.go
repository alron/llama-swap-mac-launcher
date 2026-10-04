package supervisor

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// proc identifies a process by pid plus start time, so a recycled pid is
// never mistaken for the process that used to have it.
type proc struct {
	Pid   int
	Start int64 // microseconds since the epoch
}

type procEntry struct {
	proc
	ppid   int
	name   string // executable name, truncated to 16 bytes by the kernel
	zombie bool   // exited but not yet reaped by its parent
}

// sZomb is SZOMB from <sys/proc.h>, which x/sys/unix doesn't define.
const sZomb = 5

func entryFrom(kp *unix.KinfoProc) procEntry {
	return procEntry{
		proc: proc{
			Pid:   int(kp.Proc.P_pid),
			Start: kp.Proc.P_starttime.Sec*1e6 + int64(kp.Proc.P_starttime.Usec),
		},
		ppid:   int(kp.Eproc.Ppid),
		name:   unix.ByteSliceToString(kp.Proc.P_comm[:]),
		zombie: kp.Proc.P_stat == sZomb,
	}
}

// lookup returns the process with this pid, if there is one.
func lookup(pid int) (procEntry, bool) {
	kp, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil {
		return procEntry{}, false
	}
	e := entryFrom(kp)
	return e, e.Pid == pid
}

// alive reports whether p is still running: same pid, same start time, and
// not a zombie.
func alive(p proc) bool {
	e, ok := lookup(p.Pid)
	return ok && e.Start == p.Start && !e.zombie
}

// living returns the processes in ps that are still alive.
func living(ps []proc) []proc {
	var out []proc
	for _, p := range ps {
		if alive(p) {
			out = append(out, p)
		}
	}
	return out
}

// descendants returns every process below root in the process tree.
//
// llama-swap starts each model server in its own process group, so
// signalling llama-swap's group doesn't reach them; the tree has to be
// walked by parent pid instead. Take this snapshot before signalling:
// once a process exits, its children are reparented to launchd and can no
// longer be found this way.
func descendants(root int) ([]proc, error) {
	kps, err := unix.SysctlKinfoProcSlice("kern.proc.all")
	if err != nil {
		return nil, err
	}
	all := make(map[int]procEntry, len(kps))
	children := make(map[int][]int)
	for i := range kps {
		e := entryFrom(&kps[i])
		all[e.Pid] = e
		if e.ppid != e.Pid {
			children[e.ppid] = append(children[e.ppid], e.Pid)
		}
	}

	var out []proc
	queue := []int{root}
	for len(queue) > 0 {
		pid := queue[0]
		queue = queue[1:]
		for _, c := range children[pid] {
			out = append(out, all[c].proc)
			queue = append(queue, c)
		}
	}
	return out, nil
}

// terminate sends SIGTERM to every process in ps that's still alive, waits
// up to grace for them to exit, then sends SIGKILL to any that remain.
func terminate(ps []proc, grace time.Duration) {
	signalAll(living(ps), syscall.SIGTERM)
	deadline := time.Now().Add(grace)
	for {
		left := living(ps)
		if len(left) == 0 {
			return
		}
		if time.Now().After(deadline) {
			signalAll(left, syscall.SIGKILL)
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func signalAll(ps []proc, sig syscall.Signal) {
	for _, p := range ps {
		_ = syscall.Kill(p.Pid, sig) // it may have exited already; callers check what's still alive
	}
}

func pids(ps []proc) string {
	s := make([]string, len(ps))
	for i, p := range ps {
		s[i] = strconv.Itoa(p.Pid)
	}
	return strings.Join(s, ", ")
}

// The pidfile records the running llama-swap, then its descendants, one
// "pid start name" line each, so the next launch of the app can find them
// if this one crashes. The descendants matter because llama-swap usually
// dies with the app (its output pipe breaks), leaving its model servers
// running with nothing to say whose they were.

func writePidFile(path string, e procEntry, children []procEntry) error {
	var b strings.Builder
	for _, p := range append([]procEntry{e}, children...) {
		fmt.Fprintf(&b, "%d %d %s\n", p.Pid, p.Start, p.name)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(b.String()), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// readPidFile returns llama-swap's entry and its descendants'. A file from
// before descendants were recorded has just the first line.
func readPidFile(path string) (procEntry, []procEntry, error) {
	b, err := os.ReadFile(path) // #nosec G304 -- the app's own pidfile
	if err != nil {
		return procEntry{}, nil, err
	}
	var entries []procEntry
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		var e procEntry
		if _, err := fmt.Sscanf(line, "%d %d %s", &e.Pid, &e.Start, &e.name); err != nil {
			return procEntry{}, nil, fmt.Errorf("%s: %w", path, err)
		}
		entries = append(entries, e)
	}
	return entries[0], entries[1:], nil
}

// named looks up each process's name, for the pidfile. Those that have
// exited are left out.
func named(ps []proc) []procEntry {
	var out []procEntry
	for _, p := range ps {
		if e, ok := lookup(p.Pid); ok && e.Start == p.Start {
			out = append(out, e)
		}
	}
	return out
}

// stillRunning returns the entries whose process is still alive with the
// same start time and name, so a recycled pid is never mistaken for one.
func stillRunning(entries []procEntry) []proc {
	var out []proc
	for _, want := range entries {
		if e, ok := lookup(want.Pid); ok && e.Start == want.Start && e.name == want.name && !e.zombie {
			out = append(out, want.proc)
		}
	}
	return out
}
