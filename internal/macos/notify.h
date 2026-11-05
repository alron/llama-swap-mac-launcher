// C interface to the Objective-C in notify.m. See notify.go.

// lsl_notify_setup makes the app the notification centre's delegate, so
// banners show while it's active and clicks reach it, and registers the
// categories (the GPU fault's Unload button). Call it once, early.
void lsl_notify_setup(void);

// lsl_notify_authorize asks macOS for permission to post, which shows the
// prompt the first time; it returns 1 if allowed, 0 if not, and puts any
// error, which the caller must free(), in *err.
int lsl_notify_authorize(char **err);

// lsl_notify_post posts a notification, replacing any with the same id.
// category may be empty; time_sensitive asks for the time-sensitive level,
// which needs the entitlement. It returns an error to free(), or NULL.
char *lsl_notify_post(const char *id, const char *title, const char *body, const char *category, int time_sensitive);

// lsl_notify_remove removes the notification with id, delivered or not.
void lsl_notify_remove(const char *id);

// lsl_notify_next_response waits for the next click on a notification and
// returns it as "<id>\n<action>", for the caller to free(). The action is
// "default" for a click on the notification itself.
char *lsl_notify_next_response(void);

// lsl_notify_settings returns what macOS allows the app, as JSON, for the
// caller to free().
char *lsl_notify_settings(void);
