// The GPU wired memory limit's helper (TODO item 51), for the Go code in
// wired.go: registering it as a LaunchDaemon through SMAppService, which
// the user approves in System Settings, and asking it over XPC.

#import <Metal/Metal.h>
#import <ServiceManagement/ServiceManagement.h>
#include <xpc/xpc.h>

#include "common.h"
#include "wired.h"

int lsl_helper_status(const char *plist) {
	return (int)[SMAppService daemonServiceWithPlistName:str(plist)].status;
}

char *lsl_helper_register(const char *plist, int enable) {
	SMAppService *service = [SMAppService daemonServiceWithPlistName:str(plist)];
	NSError *error = nil;
	BOOL ok = enable ? [service registerAndReturnError:&error] : [service unregisterAndReturnError:&error];
	if (ok) {
		return NULL;
	}
	const char *msg = error.localizedDescription.UTF8String;
	return strdup(msg ? msg : "unknown error");
}

char *lsl_helper_request(const char *service, const char *requirement, const char *op, long long mb, double timeout,
                         int *ok, char **error, long long *uid, long long *limit, long long *stored) {
	// Privileged: the service is a LaunchDaemon's, in the system's domain.
	xpc_connection_t conn = xpc_connection_create_mach_service(service, NULL, XPC_CONNECTION_MACH_SERVICE_PRIVILEGED);
	// The answer must come from the helper signed as the app is.
	if (xpc_connection_set_peer_code_signing_requirement(conn, requirement) != 0) {
		xpc_connection_cancel(conn);
		return strdup("the code signing requirement for the helper isn't valid");
	}
	xpc_connection_set_event_handler(conn, ^(xpc_object_t event) {
		// Replies come through send_message_with_reply_sync; nothing else.
	});
	xpc_connection_resume(conn);
	xpc_object_t message = xpc_dictionary_create(NULL, NULL, 0);
	xpc_dictionary_set_string(message, "op", op);
	xpc_dictionary_set_int64(message, "mb", mb);
	// Not send_message_with_reply_sync, which has no timeout: while launchd
	// has the job but keeps refusing to start its program (a launch
	// constraint not met, say), the message waits for it forever, and so
	// did Settings (a security review's test, 2026-11-05).
	__block xpc_object_t reply = nil;
	dispatch_semaphore_t done = dispatch_semaphore_create(0);
	xpc_connection_send_message_with_reply(conn, message, dispatch_get_global_queue(QOS_CLASS_UTILITY, 0), ^(xpc_object_t r) {
		reply = r;
		dispatch_semaphore_signal(done);
	});
	if (dispatch_semaphore_wait(done, dispatch_time(DISPATCH_TIME_NOW, (int64_t)(timeout * NSEC_PER_SEC))) != 0) {
		xpc_connection_cancel(conn); // the handler still runs, with an error, and is let go
		return strdup("the helper didn't answer: it may be registered but unable to start");
	}
	char *failed = NULL;
	if (xpc_get_type(reply) == XPC_TYPE_DICTIONARY) {
		*ok = xpc_dictionary_get_bool(reply, "ok") ? 1 : 0;
		const char *e = xpc_dictionary_get_string(reply, "error");
		*error = strdup(e != NULL ? e : "");
		*uid = xpc_dictionary_get_int64(reply, "uid");
		*limit = xpc_dictionary_get_int64(reply, "limit");
		*stored = xpc_dictionary_get_int64(reply, "stored");
	} else if (reply == XPC_ERROR_PEER_CODE_SIGNING_REQUIREMENT) {
		failed = strdup("the helper answering isn't signed as the app is");
	} else if (reply == XPC_ERROR_CONNECTION_INVALID) {
		failed = strdup("the helper isn't running: is it registered and allowed in System Settings?");
	} else {
		const char *d = xpc_dictionary_get_string(reply, XPC_ERROR_KEY_DESCRIPTION);
		failed = strdup(d != NULL ? d : "the helper didn't answer");
	}
	xpc_connection_cancel(conn);
	return failed;
}

long long lsl_gpu_working_set_mb(void) {
	id<MTLDevice> gpu = MTLCreateSystemDefaultDevice();
	return gpu != nil ? (long long)(gpu.recommendedMaxWorkingSetSize >> 20) : 0;
}
