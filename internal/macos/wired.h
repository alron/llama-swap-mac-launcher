// C interface to the Objective-C in wired.m. See wired.go.

// lsl_helper_status returns the SMAppService status of the daemon whose
// plist, in Contents/Library/LaunchDaemons, is named plist.
int lsl_helper_status(const char *plist);

// lsl_helper_register registers the daemon (enable 1) or unregisters it
// (0). It returns an error to free(), or NULL.
char *lsl_helper_register(const char *plist, int enable);

// lsl_helper_request sends the helper at the Mach service name a request
// (op, and mb), accepting an answer only from code meeting requirement,
// and waits up to timeout seconds for it. It returns an error to free() if
// the request didn't reach the helper, wasn't answered in time, or came
// back from something else; otherwise NULL, with the answer's fields.
char *lsl_helper_request(const char *service, const char *requirement, const char *op, long long mb, double timeout,
                         int *ok, char **error, long long *uid, long long *limit, long long *stored);

// lsl_gpu_working_set_mb is Metal's recommendedMaxWorkingSetSize for the
// Mac's GPU, in MB, or 0 without one: with iogpu.wired_limit_mb at 0, what
// macOS's default lets the GPU use.
long long lsl_gpu_working_set_mb(void);
