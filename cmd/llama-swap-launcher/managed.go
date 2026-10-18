package main

// The settings the app manages for llama-swap, as files in its managed
// config folder (-config-dir; TODO item 34). Each one must be a list, or
// new named entries, which add to what the user's own config sets:
// llama-swap refuses to load when two files set a single value differently.

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/alron/llama-swap-mac-launcher/internal/prefs"
)

// managedHeader starts every managed file. The supervisor only replaces a
// file that starts with the same line, so a file of the user's own with the
// same name is never overwritten.
const managedHeader = "# Written by Llama Swap Launcher, which replaces this file each time it starts llama-swap.\n" +
	"# Change it in the app's Settings instead.\n"

// managedFiles returns the managed folder's files, by name. Each is
// always there, and only the note when there's nothing in it to manage,
// so turning a setting off takes effect.
func managedFiles(p prefs.Prefs) map[string][]byte {
	return map[string][]byte{
		"api-keys.yaml": apiKeysFile(p),
		"peers.yaml":    peersFile(p),
		"security.yaml": securityFile(p),
	}
}

// apiKeysFile lists the API keys while the app manages them.
func apiKeysFile(p prefs.Prefs) []byte {
	var b strings.Builder
	b.WriteString(managedHeader)
	if p.ManagesAPIKeys() && len(p.APIKeys) > 0 {
		// The keys reach llama-swap as variables (see secretEnv), so the
		// file holds only their names. Key names are capitals, digits and
		// underscores, which need no escaping.
		b.WriteString("apiKeys:\n")
		for _, name := range p.APIKeys {
			fmt.Fprintf(&b, "  - \"${env.%s%s}\"\n", prefs.KeyPrefix, name)
		}
	}
	return []byte(b.String())
}

// peersFile lists the peers. Every name, address and model is written as a
// JSON string, which YAML reads as a quoted string, so none of them can be
// taken for YAML syntax; the preferences refuse "${" in them, which
// llama-swap would fill in. A key is the secret variable holding it, which
// llama-swap fills in from its environment.
func peersFile(p prefs.Prefs) []byte {
	var b strings.Builder
	b.WriteString(managedHeader)
	if p.ManagesPeers() {
		b.WriteString("peers:\n")
		for _, peer := range p.Peers {
			fmt.Fprintf(&b, "  %s:\n    proxy: %s\n", quoted(peer.Name), quoted(peer.URL))
			if peer.KeyVariable != "" {
				fmt.Fprintf(&b, "    apiKey: %s\n", quoted("${env."+peer.KeyVariable+"}"))
			}
			b.WriteString("    models:\n")
			for _, m := range peer.Models {
				fmt.Fprintf(&b, "      - %s\n", quoted(m))
			}
			// Only those changed from llama-swap's defaults.
			if to := peer.Timeouts; to.ResponseHeader != nil || to.Connect != nil || to.KeepAlive != nil {
				b.WriteString("    timeouts:\n")
				for _, t := range []struct {
					name string
					v    *int
				}{{"connect", to.Connect}, {"keepalive", to.KeepAlive}, {"responseHeader", to.ResponseHeader}} {
					if t.v != nil {
						fmt.Fprintf(&b, "      %s: %d\n", t.name, *t.v)
					}
				}
			}
		}
	}
	return []byte(b.String())
}

// securityFile limits which web pages may use llama-swap, while the app
// restricts them. The two switches are written only when on and maxAge
// only when changed, since each is a single value that would clash with
// a different one in the user's own config; the origins are a list, which
// adds to the user's.
func securityFile(p prefs.Prefs) []byte {
	var b strings.Builder
	b.WriteString(managedHeader)
	if !p.ManagesCORS() {
		return []byte(b.String())
	}
	c := p.CORS
	b.WriteString("security:\n  cors:\n    allowedOrigins:\n")
	for _, o := range c.AllowedOrigins {
		fmt.Fprintf(&b, "      - %s\n", quoted(o))
	}
	if c.AllowCredentials {
		b.WriteString("    allowCredentials: true\n")
	}
	if c.AllowPrivateNetwork {
		b.WriteString("    allowPrivateNetwork: true\n")
	}
	if c.MaxAge != nil {
		fmt.Fprintf(&b, "    maxAge: %d\n", *c.MaxAge)
	}
	return []byte(b.String())
}

// quoted is s as a double-quoted string that YAML reads back as s.
func quoted(s string) string {
	b, _ := json.Marshal(s) // a string always marshals
	return string(b)
}
