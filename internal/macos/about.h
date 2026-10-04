// C interface to the Objective-C in about.m. See about.go.

// lsl_show_about shows the standard About panel with version as the app's
// version, then credits (plain text, one item per line) and, if link isn't
// empty, link as a clickable address. It returns once the panel is up.
void lsl_show_about(const char *version, const char *credits, const char *link);
