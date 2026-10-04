package main

// Warnings when llama-swap can be used without an API key: the likeliest
// way a user gets hurt is listening beyond this Mac with no keys, or
// adding keys in the app but never to llama-swap's config, which then
// ignores them.

import (
	"net"

	"github.com/alron/llama-swap-mac-launcher/internal/health"
	"github.com/alron/llama-swap-mac-launcher/internal/prefs"
	"github.com/alron/llama-swap-mac-launcher/internal/supervisor"
)

// exposed reports whether llama-swap listening on listen can be reached
// from other machines: every interface, or an address that isn't loopback.
func exposed(listen string) bool {
	host, _, err := net.SplitHostPort(listen)
	if err != nil {
		return false
	}
	switch host {
	case "", "0.0.0.0", "::":
		return true
	case "localhost":
		return false
	}
	ip := net.ParseIP(host)
	return ip == nil || !ip.IsLoopback() // a host name may resolve anywhere
}

// openWarning says what an unprotected llama-swap risks, when it's worth
// saying: the app has keys that llama-swap isn't asking for, or llama-swap
// listens beyond this Mac with no key at all. A loopback llama-swap with no
// keys is the usual setup, and isn't warned about (the README says what web
// pages can do with it).
func openWarning(p prefs.Prefs, st supervisor.Status, snap health.Snapshot) string {
	if st.State != supervisor.Running || !snap.Healthy || !snap.Open {
		return ""
	}
	switch {
	case len(p.APIKeys) > 0:
		return "llama-swap isn't asking for API keys, though the app has some: its config needs " +
			`apiKeys: ["${env.` + prefs.KeyPrefix + `NAME}"]. Until then anyone who can reach it can use it`
	case exposed(st.Listen):
		return "llama-swap is open to the network without an API key: anyone who can reach this Mac " +
			"can use it, unload its models and read its logs. Add a key under Settings… → Secrets"
	}
	return ""
}

// opensWithoutKey reports whether going from old to p opens llama-swap to
// other machines with no API key, so the Save is worth confirming. It's
// asked once, when that starts, not on every Save after.
func opensWithoutKey(old, p prefs.Prefs) bool {
	open := func(q prefs.Prefs) bool { return exposed(q.ListenAddr()) && len(q.APIKeys) == 0 }
	return open(p) && !open(old)
}
