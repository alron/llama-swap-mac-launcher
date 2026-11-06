// C interface to the Objective-C in codesign.m. See codesign.go.

// lsl_capture_self_requirement keeps the running app's designated
// requirement (its bundle ID and signing team), read from the running code
// itself. It returns an error to free(), or NULL.
char *lsl_capture_self_requirement(void);

// lsl_matches_self checks the app at path: signed, sealed and unmodified,
// and meeting the requirement lsl_capture_self_requirement kept. It returns
// an error to free(), or NULL if it does.
char *lsl_matches_self(const char *path);
