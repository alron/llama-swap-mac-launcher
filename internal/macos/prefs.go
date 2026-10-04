package macos

/*
#cgo CFLAGS: -fobjc-arc
#cgo LDFLAGS: -framework AppKit
#include <stdlib.h>
#include "prefs.h"
*/
import "C"

import (
	"encoding/json"
	"errors"
	"fmt"
	"unsafe"
)

// PreferencesForm is what the Preferences dialog shows and edits: the
// preferences as text, as they appear in its fields. Checking the text is
// up to the caller.
type PreferencesForm struct {
	Binary                    string `json:"binary"`
	Config                    string `json:"config"`
	Listen                    string `json:"listen"`
	Args                      string `json:"args"` // one per line
	Env                       string `json:"env"`  // NAME=value, one per line
	HealthCheckSeconds        string `json:"healthCheckSeconds"`
	HealthCheckSkipWhenActive bool   `json:"healthCheckSkipWhenActive"`
	UnloadOnGPUFault          bool   `json:"unloadOnGpuFault"`
	MarkUpdatedModels         bool   `json:"markUpdatedModels"`
	AutoStart                 bool   `json:"autoStart"`
	// The Secrets tab: the API keys and the name of the one the app uses,
	// and the secret environment variables.
	APIKeys   []SecretField `json:"apiKeys"`
	AppAPIKey string        `json:"appApiKey"`
	SecretEnv []SecretField `json:"secretEnv"`
	// The Logs tab.
	LauncherLog      string `json:"launcherLog"`
	LlamaSwapLog     string `json:"llamaSwapLog"` // "launcher", "file" or "off"
	LlamaSwapLogFile string `json:"llamaSwapLogFile"`
	// Tab is the tab to show first, "general", "secrets" or "logs", and
	// comes back as the one showing when the dialog closed.
	Tab string `json:"tab"`

	// Shown but not edited:
	FoundBinary string `json:"foundBinary"` // greyed out in the empty binary field
	File        string `json:"file"`        // the preferences file
	Error       string `json:"error"`       // why the last Save didn't work
	KeyPrefix   string `json:"keyPrefix"`   // shown before each key's name
	// The logs' default paths, greyed out in their empty fields.
	DefaultLauncherLog  string `json:"defaultLauncherLog"`
	DefaultLlamaSwapLog string `json:"defaultLlamaSwapLog"`
}

// SecretField is one row of a table on the Secrets tab: the name (for an
// API key, without the prefix) and the secret value.
type SecretField struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// PreferencesResult says how the dialog was closed.
type PreferencesResult int

const (
	PreferencesCancel PreferencesResult = iota // Cancel, or closed by CloseDialogs
	PreferencesSave
)

// Preferences shows the Preferences dialog, filled in from form, and returns
// the fields as the user left them and how they closed it. An error means
// the dialog's reply couldn't be read; report it, since treating it as a
// Cancel would silently lose the user's changes.
func Preferences(form PreferencesForm) (PreferencesForm, PreferencesResult, error) {
	in, err := json.Marshal(form)
	if err != nil {
		return form, PreferencesCancel, err
	}
	cin := cString(string(in))
	defer C.free(unsafe.Pointer(cin))
	var result C.int
	cout := C.lsl_preferences(cin, &result)
	if cout == nil {
		return form, PreferencesCancel, errors.New("the Settings window returned nothing")
	}
	defer C.free(unsafe.Pointer(cout))
	out := form // keep the fields the dialog doesn't return
	if err := json.Unmarshal([]byte(C.GoString(cout)), &out); err != nil {
		return form, PreferencesCancel, fmt.Errorf("reading the Settings window's reply: %w", err)
	}
	return out, PreferencesResult(result), nil
}
