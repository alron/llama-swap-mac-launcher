// Command llama-swap-launcher is the menu-bar app that runs, supervises and
// monitors llama-swap. See CLAUDE.md for why it exists.
package main

import (
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"fyne.io/systray"

	"github.com/alron/llama-swap-mac-launcher/internal/control"
	"github.com/alron/llama-swap-mac-launcher/internal/paths"
	"github.com/alron/llama-swap-mac-launcher/internal/prefs"
)

// Set by the Makefile.
var (
	bundleID  = "com.my-wang.llama-swap-launcher.dev"
	version   = "dev"
	buildDate = "" // YYYY-MM-DD, UTC
)

// homepage is the project's page, linked from the About panel.
const homepage = "https://github.com/alron/llama-swap-mac-launcher"

func main() {
	p, err := paths.New(bundleID)
	if err != nil {
		fatal(err)
	}
	if err := p.Create(); err != nil {
		fatal(err)
	}
	// The logs first, so everything after is recorded. The preferences say
	// where they go (Load gives the defaults if it can't read them; onReady
	// reads them again and reports why).
	pr, _ := prefs.Load(p.Prefs())
	lg := newLogs(p.LogDir)
	logErr := lg.apply(pr)
	if logErr != nil {
		_ = lg.apply(prefs.Defaults()) // the default places, in the app's own folder
	}
	defer lg.close()

	a := newApp(p, lg)
	if logErr != nil {
		a.logErr = fmt.Errorf("%w. Logging to the default places until that's fixed in Settings… → Logs", logErr)
	}
	a.sup.Logf("Llama Swap Launcher %s (%s) starting, pid %d", version, bundleID, os.Getpid())

	// The control socket is also the single-instance lock, so take it
	// before anything else: CleanupLeftovers would otherwise stop another
	// instance's llama-swap.
	l, err := control.Listen(p.Socket())
	switch {
	case errors.Is(err, control.ErrAlreadyRunning):
		a.sup.Logf("another instance is already running; exiting")
		os.Exit(0)
	case err != nil:
		// The socket is also the lock that keeps a second copy of the app
		// from stopping or doubling the first one's llama-swap. Without it,
		// nothing is cleaned up or started automatically.
		a.startupErr = fmt.Errorf("llsl won't be able to reach the app: %w. "+
			"To be safe, llama-swap isn't started automatically, nor anything left from a previous run stopped: "+
			"another copy of the app might be running it. Start it from the menu if not", err)
	default:
		a.ctl = l
		go control.Serve(l, a.handle)
	}

	if a.ctl != nil {
		a.sup.CleanupLeftovers()
	}

	// Stop llama-swap on kill or Ctrl-C too, not just on Quit.
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sigs
		a.quit()
	}()

	systray.Run(a.onReady, a.onExit)
	a.shutdown()
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "llama-swap-launcher:", err)
	os.Exit(1)
}
