package main

// Secrets: llama-swap's API keys and secret environment variables. Their
// names are in the preferences and their values in the login keychain.
// Both reach llama-swap as environment variables, so each is stored under
// the variable's name: LLSL_ADMIN for the API key ADMIN (its config uses it
// as "${env.LLSL_ADMIN}"), HF_TOKEN for that secret variable. The names
// can't clash, since only API keys start with prefs.KeyPrefix.

import (
	"errors"
	"fmt"
	"strings"

	"github.com/alron/llama-swap-mac-launcher/internal/macos"
	"github.com/alron/llama-swap-mac-launcher/internal/prefs"
)

// secretVariables returns the variables p keeps secret: its API keys, then
// its secret environment variables.
func secretVariables(p prefs.Prefs) []string {
	var vars []string
	for _, name := range p.APIKeys {
		vars = append(vars, prefs.KeyPrefix+name)
	}
	return append(vars, p.SecretEnv...)
}

// secretEnv reads p's secrets from the keychain, as variables for
// llama-swap, and returns them with the value of the API key the app sends
// on its own requests ("" if there are no keys). A secret without a value
// is an error, since llama-swap would start without it.
func secretEnv(p prefs.Prefs) (env map[string]string, appKey string, err error) {
	env = map[string]string{}
	for _, v := range secretVariables(p) {
		value, err := macos.KeychainGet(bundleID, v)
		if errors.Is(err, macos.ErrNotInKeychain) {
			return nil, "", fmt.Errorf("the %s has no value in the keychain. Set one in Settings… → Secrets", describeSecret(v))
		}
		if err != nil {
			return nil, "", fmt.Errorf("reading the %s: %w", describeSecret(v), err)
		}
		env[v] = value
	}
	if name := p.AppKey(); name != "" {
		appKey = env[prefs.KeyPrefix+name]
	}
	return env, appKey, nil
}

// readSecrets returns the values of p's secrets by variable, "" for any
// that has none in the keychain.
func readSecrets(p prefs.Prefs) (map[string]string, error) {
	secrets := map[string]string{}
	for _, v := range secretVariables(p) {
		value, err := macos.KeychainGet(bundleID, v)
		if err != nil && !errors.Is(err, macos.ErrNotInKeychain) {
			return nil, fmt.Errorf("reading the %s: %w", describeSecret(v), err)
		}
		secrets[v] = value
	}
	return secrets, nil
}

// saveSecrets brings the keychain from old to secrets (both by variable):
// it stores those that are new or changed, and deletes those that are gone.
// Items are found by the app's bundle ID, so dev and release builds keep
// their own.
func saveSecrets(old, secrets map[string]string) error {
	for v, value := range secrets {
		if old[v] != value {
			label := "Llama Swap Launcher: " + describeSecret(v)
			if err := macos.KeychainSet(bundleID, v, label, value); err != nil {
				return fmt.Errorf("storing the %s: %w", describeSecret(v), err)
			}
		}
	}
	for v := range old {
		if _, ok := secrets[v]; !ok {
			if err := macos.KeychainDelete(bundleID, v); err != nil {
				return fmt.Errorf("removing the %s: %w", describeSecret(v), err)
			}
		}
	}
	return nil
}

// describeSecret names a secret by its variable, for messages and the
// keychain item's label.
func describeSecret(v string) string {
	if strings.HasPrefix(v, prefs.KeyPrefix) {
		return "llama-swap API key " + v
	}
	return "secret environment variable " + v
}
