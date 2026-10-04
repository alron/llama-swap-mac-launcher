package main

// Collect Diagnostics…: a zip for bug reports, with what whoever helps
// would ask for, and secrets redacted (see internal/diagnostics). Nothing
// is sent anywhere; the user decides where it goes.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"os/user"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"golang.org/x/sys/unix"

	"github.com/alron/llama-swap-mac-launcher/internal/diagnostics"
	"github.com/alron/llama-swap-mac-launcher/internal/health"
	"github.com/alron/llama-swap-mac-launcher/internal/macos"
	"github.com/alron/llama-swap-mac-launcher/internal/prefs"
	"github.com/alron/llama-swap-mac-launcher/internal/supervisor"
)

// collectDiagnostics asks where to save the zip, writes it, and shows it
// in the Finder. Like Preferences, it keeps the menu to Quit meanwhile.
func (a *app) collectDiagnostics() {
	if !a.inDialog.CompareAndSwap(false, true) {
		return
	}
	a.render()
	defer func() {
		a.inDialog.Store(false)
		a.render()
	}()
	home, _ := os.UserHomeDir()
	name := "Llama Swap Launcher diagnostics " + time.Now().Format("2006-01-02 15.04") + ".zip"
	path := macos.SaveFile("Save the diagnostics, for a bug report", filepath.Join(home, "Desktop"), name)
	if path == "" {
		return
	}
	files, err := a.diagnosticFiles()
	if err != nil {
		a.problem("Couldn't collect the diagnostics", err)
		return
	}
	var b bytes.Buffer
	if err := diagnostics.Zip(&b, strings.TrimSuffix(filepath.Base(path), ".zip"), files); err != nil {
		a.problem("Couldn't collect the diagnostics", err)
		return
	}
	if err := os.WriteFile(path, b.Bytes(), 0o600); err != nil {
		a.problem("Couldn't save the diagnostics", err)
		return
	}
	a.sup.Logf("diagnostics saved in %s", path)
	a.reveal(path)
	macos.Alert("Diagnostics saved",
		"They're shown in the Finder. Keys, tokens and passwords are replaced with "+diagnostics.Redacted+
			", your home folder with ~ and your name with <user>. Hostnames, addresses and paths stay, and the logs "+
			"can hold prompts and replies. Look the files over before you share them: no redaction can promise "+
			"to catch everything. Nothing has been sent anywhere.")
}

// diagnosticFiles gathers the zip's files, redacted.
func (a *app) diagnosticFiles() ([]diagnostics.File, error) {
	p := a.currentPrefs()
	secrets, err := readSecrets(p)
	if err != nil {
		return nil, err
	}
	// Every known secret's exact value goes, wherever it turns up.
	r := diagnostics.NewRedactor(slices.Collect(maps.Values(secrets))...)
	home, _ := os.UserHomeDir()
	username := ""
	if u, err := user.Current(); err == nil {
		username = u.Username
	}
	clean := func(b []byte) []byte {
		return []byte(r.Text(diagnostics.Anonymize(string(b), home, username)))
	}
	var files []diagnostics.File
	add := func(name string, data []byte) {
		files = append(files, diagnostics.File{Name: name, Data: clean(data)})
	}

	add("summary.txt", []byte(a.diagnosticSummary(p, secrets)))
	if status, err := json.MarshalIndent(a.controlStatus(), "", "  "); err == nil {
		add("status.json", status)
	}
	// The preferences as the app reads them, with every env value blanked,
	// whatever its name: the app knows that map exactly. A file that
	// doesn't load goes in as it is (redacted), since that's what needs
	// looking at.
	if _, err := prefs.Load(a.paths.Prefs()); err != nil {
		if b, err := os.ReadFile(a.paths.Prefs()); err == nil { // #nosec G304 -- the app's own preferences
			add("prefs.json (as on disk; it doesn't load)", b)
		}
	} else {
		shown := p
		shown.Env = map[string]string{}
		for name := range p.Env {
			shown.Env[name] = diagnostics.Redacted
		}
		if b, err := json.MarshalIndent(shown, "", "  "); err == nil { // #nosec G117 -- APIKeys holds names only, and env values are blanked above
			add("prefs.json", b)
		}
	}
	if config := p.ConfigPath(); config != "" {
		if b, err := os.ReadFile(config); err == nil { // #nosec G304 -- the user's llama-swap config, from the preferences
			add("llama-swap config/"+filepath.Base(config), b)
		}
	}
	launcher, output := a.logs.paths()
	logs := []string{launcher}
	if output != "" && output != launcher {
		logs = append(logs, output)
	}
	for _, log := range logs {
		for _, f := range []string{log, log + ".1"} { // the current file and the one before
			if b, err := os.ReadFile(f); err == nil { // #nosec G304 -- the app's own logs, from its preferences
				add("logs/"+filepath.Base(f), b)
			}
		}
	}
	add("llama-swap output (in memory).txt", []byte(strings.Join(a.sup.RecentOutput(100), "\n")+"\n"))
	return files, nil
}

// diagnosticSummary is what a bug report needs first: versions, hardware,
// and how the app is set up. Secrets appear by name only.
func (a *app) diagnosticSummary(p prefs.Prefs, secrets map[string]string) string {
	var b strings.Builder
	line := func(label, format string, args ...any) {
		fmt.Fprintf(&b, "%-17s %s\n", label+":", fmt.Sprintf(format, args...))
	}
	fmt.Fprintf(&b, "Llama Swap Launcher diagnostics, collected %s\n\n", time.Now().Format("2006-01-02 15:04:05 -0700"))
	line("App", "%s, built %s (%s)", version, orUnknown(buildDate), bundleID)
	osVersion, _ := unix.Sysctl("kern.osproductversion")
	osBuild, _ := unix.Sysctl("kern.osversion")
	line("macOS", "%s (%s)", orUnknown(osVersion), orUnknown(osBuild))
	model, _ := unix.Sysctl("hw.model")
	chip, _ := unix.Sysctl("machdep.cpu.brand_string")
	memory, _ := unix.SysctlUint64("hw.memsize")
	line("Hardware", "%s, %s, %d GB", orUnknown(model), orUnknown(chip), memory>>30)

	st := a.sup.Status()
	bin := p.BinaryPath()
	if bin == "" {
		home, _ := os.UserHomeDir()
		bin = supervisor.FindBinary("llama-swap", supervisor.SearchDirs(home))
	}
	v := st.Version
	if v == "" && bin != "" {
		v = supervisor.BinaryVersion(bin)
	}
	line("llama-swap", "%s at %s (tested with v%d; v%d at least)", orUnknown(v), orUnknown(bin), health.TestedVersion, health.MinimumVersion)
	line("State", "%s", describe(st, a.snapshot()))
	if msg := message(st, a.snapshot()); msg != "" {
		line("", "%s", msg)
	}
	line("Config", "%s", orUnknown(p.ConfigPath()))
	line("Listen", "%s", p.ListenAddr())
	launcher, output := a.logs.paths()
	line("Logs", "launcher %s; llama-swap's output %s", launcher, describeOutput(p.LogMode(), output))
	line("Launch at login", "%v", macos.LoginItem() == macos.LoginItemEnabled)
	a.mu.Lock()
	fault := a.lastFault
	a.mu.Unlock()
	if fault != nil {
		line("Last GPU fault", "%s", faultLine(fault, time.Now()))
	}
	names := slices.Sorted(maps.Keys(secrets))
	// Not labelled "Secrets": the redaction would take that for a secret.
	line("In the keychain", "%d, by name only: %s", len(names), orUnknown(strings.Join(names, ", ")))
	fmt.Fprintf(&b, "\nKeys, tokens and passwords are replaced with %s in every file here, the home folder with ~, and the user's name with <user>.\n", diagnostics.Redacted)
	return b.String()
}

func describeOutput(mode, path string) string {
	switch mode {
	case prefs.LogOff:
		return "not saved"
	case prefs.LogToFile:
		return "in " + path
	}
	return "in the launcher log"
}

func orUnknown(s string) string {
	if s == "" {
		return "(none)"
	}
	return s
}
