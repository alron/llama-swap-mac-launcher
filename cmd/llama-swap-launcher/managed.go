package main

// The settings the app manages for llama-swap, as files in its managed
// config folder (-config-dir; TODO item 34). Each one must be a list, or
// new named entries, which add to what the user's own config sets:
// llama-swap refuses to load when two files set a single value differently.

import (
	"fmt"
	"strings"

	"github.com/alron/llama-swap-mac-launcher/internal/prefs"
)

// managedHeader starts every managed file. The supervisor only replaces a
// file that starts with the same line, so a file of the user's own with the
// same name is never overwritten.
const managedHeader = "# Written by Llama Swap Launcher, which replaces this file each time it starts llama-swap.\n" +
	"# Change it in the app's Settings instead.\n"

// managedFiles returns the managed folder's files, by name. api-keys.yaml
// is always there: it lists the API keys while the app manages them, and
// is otherwise only the note, so turning the setting off takes effect.
func managedFiles(p prefs.Prefs) map[string][]byte {
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
	return map[string][]byte{"api-keys.yaml": []byte(b.String())}
}
