// C interface to the Objective-C in prefs.m. See prefs.go.

// lsl_preferences shows the Preferences dialog, filled in from json (the
// fields of PreferencesForm in prefs.go), and waits for it to close. It
// returns the fields as the user left them, as JSON the caller must free(),
// and sets *result to 1 for Save or 0 for Cancel.
char *lsl_preferences(const char *json, int *result);

// lsl_fetch_peer_models is Go (peers.go), called by the dialog's Fetch
// models off the main thread: it asks the peer at url for its models,
// sending key if it isn't empty, and returns {"models": [...], "skipped": n}
// or {"error": "..."} as JSON the caller must free().
char *lsl_fetch_peer_models(char *url, char *key);
