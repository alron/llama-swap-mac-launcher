// Command localnet-spike is milestone 0: it checks that macOS attributes
// Local Network access by this app's child processes, llama-swap included,
// to the app itself.
//
// Launched with `open`, it runs as a menu-bar app (like the real launcher
// will), runs the probes, and shows the results in its menu and in the log
// file. Run directly with -direct, it runs the probes once without retrying
// and prints the results. That's the negative control: whoever ran it
// (tmux, say) is then the responsible process, not the app.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"fyne.io/systray"
)

const (
	bundleID   = "com.my-wang.llama-swap-launcher.dev"
	swapListen = "127.0.0.1:18080"

	// peerConfig is a llama-swap config with no local models and one peer.
	// A request for "lanpeer/<model>" makes llama-swap connect to the peer.
	peerConfig = `peers:
  lanpeer:
    proxy: http://%s
    models:
      - %s
`
)

var (
	target   = flag.String("target", "peer.local:8080", "LAN `host:port` to connect to: another llama-swap, for the llama-swap probe")
	swapBin  = flag.String("llama-swap", "/opt/homebrew/bin/llama-swap", "llama-swap binary")
	model    = flag.String("peer-model", "gemma4-26b-a4b-q8", "model to request from the peer; should already be loaded there")
	direct   = flag.Bool("direct", false, "run the probes once, print the results and exit, without the menu bar")
	child    = flag.Bool("child", false, "internal: run as the Go child probe")
	retryFor = flag.Duration("retry", 90*time.Second, "how long to keep retrying blocked probes (time to answer the permission prompt)")

	// buildStamp makes every build a different binary, so rebuilding tests
	// whether the permission survives a new build. Set by build.sh.
	buildStamp = "unset"

	logw io.Writer = os.Stdout
)

// outcome is what a probe learned about the path to the LAN host. Higher
// values are worse.
type outcome int

const (
	pending outcome = iota
	reached         // got to the host: connected, or refused by the host itself
	failed          // something else, such as a timeout or DNS failure
	blocked         // "no route to host", which is what Local Network privacy produces
)

func (o outcome) String() string {
	switch o {
	case reached:
		return "REACHED"
	case failed:
		return "FAILED"
	case blocked:
		return "BLOCKED"
	}
	return "..."
}

type probe struct {
	name string
	run  func(context.Context) (outcome, string)
}

func main() {
	flag.Parse()
	switch {
	case *child:
		os.Exit(int(childProbe()))
	case *direct:
		logHeader("direct")
		if runAll(0, func(int, outcome, string) {}) != reached {
			os.Exit(1)
		}
	default:
		f, err := openLog()
		if err == nil {
			defer f.Close()
			logw = io.MultiWriter(os.Stdout, f)
		}
		logHeader("menu bar")
		systray.Run(onReady, func() {})
	}
}

func onReady() {
	systray.SetTitle("LN spike")
	systray.SetTooltip("Llama Swap Launcher Dev: Local Network test")

	names := probes(nil)
	items := make([]*systray.MenuItem, len(names))
	for i, p := range names {
		items[i] = systray.AddMenuItem(p.name+": ...", "")
		items[i].Disable()
	}
	systray.AddSeparator()
	again := systray.AddMenuItem("Run again", "")
	quit := systray.AddMenuItem("Quit", "")

	var running sync.Mutex
	run := func() {
		if !running.TryLock() {
			return
		}
		defer running.Unlock()
		runAll(*retryFor, func(i int, o outcome, detail string) {
			items[i].SetTitle(fmt.Sprintf("%s  %s: %s", o, names[i].name, truncate(detail, 70)))
		})
	}

	go run()
	go func() {
		for {
			select {
			case <-again.ClickedCh:
				go run()
			case <-quit.ClickedCh:
				systray.Quit()
				return
			}
		}
	}()
}

// probes lists the checks in order. The first LAN connection comes from a
// child process, so if a permission prompt appears, it shows whose name
// macOS puts on a child's connection.
func probes(swapErr error) []probe {
	return []probe{
		{"child /usr/bin/nc", ncChild},
		{"child Go binary (+ grandchild nc)", goChild},
		{"child llama-swap -> peer", func(ctx context.Context) (outcome, string) {
			if swapErr != nil {
				return failed, swapErr.Error()
			}
			return swapPeer(ctx)
		}},
		{"app itself", dialTarget},
	}
}

// runAll starts llama-swap, runs every probe, and returns the worst outcome.
// A blocked probe is retried every few seconds until retryFor has passed,
// which leaves time to answer the permission prompt.
func runAll(retryFor time.Duration, update func(i int, o outcome, detail string)) outcome {
	ctx := context.Background()
	logf("--- run (target %s)", *target)

	stop, swapErr := startLlamaSwap(ctx)
	if swapErr == nil {
		defer stop()
	}

	worst := pending
	deadline := time.Now().Add(retryFor)
	for i, p := range probes(swapErr) {
		for {
			o, detail := p.run(ctx)
			logf("%-8s %s: %s", o, p.name, detail)
			update(i, o, detail)
			if o != blocked || time.Now().After(deadline) {
				worst = max(worst, o)
				break
			}
			time.Sleep(3 * time.Second)
		}
	}
	logf("--- worst outcome: %s", worst)
	return worst
}

// dialTarget connects from the current process.
func dialTarget(ctx context.Context) (outcome, string) {
	d := net.Dialer{Timeout: 5 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", *target)
	if err != nil {
		return classifyErr(err), err.Error()
	}
	defer conn.Close()
	return reached, "connected to " + conn.RemoteAddr().String()
}

// ncChild connects from a child process running Apple's nc.
func ncChild(ctx context.Context) (outcome, string) {
	host, port, err := net.SplitHostPort(*target)
	if err != nil {
		return failed, err.Error()
	}
	out, _ := exec.CommandContext(ctx, "/usr/bin/nc", "-z", "-v", "-G", "5", host, port).CombinedOutput()
	s := strings.TrimSpace(string(out))
	return classifyText(s), s
}

// goChild runs this binary again in -child mode, which connects itself and
// then runs nc as a grandchild. Its exit code is its worst outcome.
func goChild(ctx context.Context) (outcome, string) {
	self, err := os.Executable()
	if err != nil {
		return failed, err.Error()
	}
	cmd := exec.CommandContext(ctx, self, "-child", "-target", *target)
	out, err := cmd.CombinedOutput()
	if cmd.ProcessState == nil {
		return failed, err.Error()
	}
	detail := strings.Join(strings.Fields(string(out)), " ")
	o := outcome(cmd.ProcessState.ExitCode())
	if o < reached || o > blocked {
		return failed, fmt.Sprintf("exit %d: %s", cmd.ProcessState.ExitCode(), detail)
	}
	return o, detail
}

func childProbe() outcome {
	o1, d1 := dialTarget(context.Background())
	o2, d2 := ncChild(context.Background())
	fmt.Printf("self %s (%s); grandchild nc %s (%s)\n", o1, d1, o2, d2)
	return max(o1, o2)
}

// startLlamaSwap runs llama-swap as a direct child with the peer-only
// config and waits until it answers /health.
func startLlamaSwap(ctx context.Context) (stop func(), err error) {
	dir, err := os.MkdirTemp("", "localnet-spike")
	if err != nil {
		return nil, err
	}
	cfg := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfg, fmt.Appendf(nil, peerConfig, *target, *model), 0o600); err != nil {
		os.RemoveAll(dir)
		return nil, err
	}

	cmd := exec.Command(*swapBin, "-config", cfg, "-listen", swapListen)
	cmd.Stdout, cmd.Stderr = logw, logw
	if err := cmd.Start(); err != nil {
		os.RemoveAll(dir)
		return nil, fmt.Errorf("start llama-swap: %w", err)
	}
	stop = func() {
		stopProcess(cmd)
		os.RemoveAll(dir)
	}

	deadline := time.Now().Add(15 * time.Second)
	for {
		resp, err := http.Get("http://" + swapListen + "/health")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return stop, nil
			}
		}
		if time.Now().After(deadline) {
			stop()
			return nil, fmt.Errorf("llama-swap didn't become healthy on %s", swapListen)
		}
		time.Sleep(250 * time.Millisecond)
	}
}

// swapPeer asks the local llama-swap for the peer's model, so llama-swap
// makes the LAN connection. A 502 is llama-swap's own proxy error; any
// other status came back from the peer.
func swapPeer(ctx context.Context) (outcome, string) {
	body := fmt.Sprintf(`{"model":"lanpeer/%s","messages":[{"role":"user","content":"hi"}],"max_tokens":1}`, *model)
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"http://"+swapListen+"/v1/chat/completions", strings.NewReader(body))
	if err != nil {
		return failed, err.Error()
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return failed, err.Error()
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 500))
	detail := fmt.Sprintf("HTTP %d: %s", resp.StatusCode, strings.Join(strings.Fields(string(b)), " "))
	if resp.StatusCode == http.StatusBadGateway {
		return classifyText(string(b)), detail
	}
	return reached, detail
}

func stopProcess(cmd *exec.Cmd) {
	done := make(chan struct{})
	go func() {
		cmd.Wait()
		close(done)
	}()
	cmd.Process.Signal(syscall.SIGTERM)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		cmd.Process.Kill()
		<-done
	}
}

func classifyErr(err error) outcome {
	switch {
	case errors.Is(err, syscall.ECONNREFUSED):
		return reached
	case errors.Is(err, syscall.EHOSTUNREACH):
		return blocked
	}
	return failed
}

func classifyText(s string) outcome {
	s = strings.ToLower(s)
	switch {
	case strings.Contains(s, "no route to host"):
		return blocked
	case strings.Contains(s, "succeeded"), strings.Contains(s, "connection refused"):
		return reached
	}
	return failed
}

func openLog() (*os.File, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(home, "Library", "Logs", bundleID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return os.OpenFile(filepath.Join(dir, "spike.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
}

func logHeader(mode string) {
	exe, _ := os.Executable()
	logf("=== localnet-spike, %s mode, build %s, pid %d, parent pid %d", mode, buildStamp, os.Getpid(), os.Getppid())
	logf("    executable %s", exe)
}

func logf(format string, args ...any) {
	fmt.Fprintf(logw, time.Now().Format("15:04:05 ")+format+"\n", args...)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
