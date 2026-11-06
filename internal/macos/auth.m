// Confirming a change with the person at the Mac (TODO item 51), for the Go
// code in auth.go: LocalAuthentication's device owner check, which asks for
// Touch ID or a paired Apple Watch, falling back to the login password. The
// check happens in the Secure Enclave and the system's own window; the app
// learns only whether it passed.

#import <LocalAuthentication/LocalAuthentication.h>

#include "auth.h"
#include "common.h"

// The confirmation in progress, if any, for lsl_cancel_confirm_owner.
static LAContext *current;

int lsl_confirm_owner(const char *reason, char **error) {
	*error = NULL;
	LAContext *context = [[LAContext alloc] init];
	NSError *e = nil;
	if (![context canEvaluatePolicy:LAPolicyDeviceOwnerAuthentication error:&e]) {
		*error = strdup((e.localizedDescription ?: @"this Mac can't confirm it").UTF8String);
		return 2;
	}
	@synchronized([LAContext class]) {
		current = context;
	}
	// The system shows its own window and calls back on a queue of its
	// own; this waits for it, off the main thread (Go calls it from a
	// goroutine).
	dispatch_semaphore_t done = dispatch_semaphore_create(0);
	__block int result = 2;
	__block NSString *why = nil;
	[context evaluatePolicy:LAPolicyDeviceOwnerAuthentication
	        localizedReason:str(reason)
	                  reply:^(BOOL ok, NSError *err) {
		                  if (ok) {
			                  result = 0;
		                  } else if (err.code == LAErrorUserCancel || err.code == LAErrorSystemCancel ||
		                             err.code == LAErrorAppCancel) {
			                  result = 1;
		                  } else {
			                  why = err.localizedDescription ?: @"it couldn't be confirmed";
		                  }
		                  dispatch_semaphore_signal(done);
	                  }];
	dispatch_semaphore_wait(done, DISPATCH_TIME_FOREVER);
	@synchronized([LAContext class]) {
		current = nil;
	}
	if (why != nil) {
		*error = strdup(why.UTF8String);
	}
	return result;
}

void lsl_cancel_confirm_owner(void) {
	@synchronized([LAContext class]) {
		[current invalidate]; // its reply comes back as LAErrorAppCancel
	}
}
