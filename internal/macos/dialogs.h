// C interface to the Objective-C in dialogs.m. See dialogs.go.

// lsl_choose_file shows an Open panel, for a file or, with dirs, a file or
// a folder. It returns the chosen path, which the caller must free(), or
// NULL if the user cancelled.
char *lsl_choose_file(const char *title, const char *start_dir, int show_hidden, int dirs);

// lsl_save_file shows a Save panel starting in dir with name filled in. It
// returns the chosen path, which the caller must free(), or NULL if the
// user cancelled.
char *lsl_save_file(const char *title, const char *dir, const char *name);

// lsl_alert shows a modal alert with an OK button. If output isn't empty,
// it's shown under the message in a scrollable box.
void lsl_alert(const char *message, const char *detail, const char *output);

// lsl_confirm shows a modal alert with two buttons and returns 1 if the
// user chose the first (ok_title), 0 otherwise.
int lsl_confirm(const char *message, const char *detail, const char *ok_title, const char *cancel_title);

// lsl_close_dialogs is for quitting: it closes every dialog of ours that's
// open, innermost first, as if each were cancelled, and any shown after it
// returns at once as cancelled. It waits up to timeout seconds for them to
// close and returns 1 if they did. Don't call it on the main thread.
int lsl_close_dialogs(double timeout);
