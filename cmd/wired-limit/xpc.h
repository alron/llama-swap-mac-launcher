// C interface to xpc.c, the helper's XPC service. See main.go.

// lsl_helper_listen starts the Mach service name, accepting only peers that
// meet requirement (a code signing requirement), and returns. Requests then
// queue for lsl_helper_next. It returns an error to free(), or NULL.
char *lsl_helper_listen(const char *name, const char *requirement);

// lsl_helper_next waits for the next request and returns its handle, with
// its "op" and "mb"; *op is the caller's to free().
void *lsl_helper_next(char **op, long long *mb);

// lsl_helper_reply answers the request handle, and releases it.
void lsl_helper_reply(void *handle, int ok, const char *error, long long uid, long long limit, long long stored);

// lsl_set_wired_limit_mb sets iogpu.wired_limit_mb, which needs root, and
// returns errno, or 0.
int lsl_set_wired_limit_mb(int mb);
