// C interface to the Objective-C in prefs.m. See prefs.go.

// lsl_preferences shows the Preferences dialog, filled in from json (the
// fields of PreferencesForm in prefs.go), and waits for it to close. It
// returns the fields as the user left them, as JSON the caller must free(),
// and sets *result to 1 for Save or 0 for Cancel.
char *lsl_preferences(const char *json, int *result);
