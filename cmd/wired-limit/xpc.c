// The helper's XPC service (TODO item 51): it listens on its Mach service,
// which only the signed app may use, and hands each request to Go, which
// answers it. XPC's handlers are blocks, which Go can't write, so they stay
// here; Go waits for requests in lsl_helper_next rather than being called.
// Plain C, not Objective-C: XPC objects are retained and released by hand.

#include <errno.h>
#include <pthread.h>
#include <stdlib.h>
#include <string.h>
#include <sys/sysctl.h>
#include <xpc/xpc.h>

#include "xpc.h"

// A request waiting for Go: the message (for its reply) and its sender.
typedef struct request {
	xpc_object_t message;
	xpc_connection_t peer;
	struct request *next;
} request;

static pthread_mutex_t lock = PTHREAD_MUTEX_INITIALIZER;
static request *first, *last;
static dispatch_semaphore_t waiting;
static xpc_connection_t listener;

static void enqueue(xpc_connection_t peer, xpc_object_t message) {
	request *r = calloc(1, sizeof *r);
	r->message = xpc_retain(message);
	r->peer = xpc_retain(peer);
	pthread_mutex_lock(&lock);
	if (last != NULL) {
		last->next = r;
	} else {
		first = r;
	}
	last = r;
	pthread_mutex_unlock(&lock);
	dispatch_semaphore_signal(waiting);
}

char *lsl_helper_listen(const char *name, const char *requirement) {
	waiting = dispatch_semaphore_create(0);
	listener = xpc_connection_create_mach_service(name, NULL, XPC_CONNECTION_MACH_SERVICE_LISTENER);
	// On the listener, it applies to every peer: a request from anything
	// else is dropped before it reaches the handler.
	if (xpc_connection_set_peer_code_signing_requirement(listener, requirement) != 0) {
		return strdup("the code signing requirement for the app isn't valid");
	}
	xpc_connection_set_event_handler(listener, ^(xpc_object_t peer) {
		if (xpc_get_type(peer) != XPC_TYPE_CONNECTION) {
			return; // an error on the listener itself
		}
		xpc_connection_set_event_handler(peer, ^(xpc_object_t event) {
			if (xpc_get_type(event) == XPC_TYPE_DICTIONARY) {
				enqueue(peer, event);
			}
		});
		xpc_connection_resume(peer);
	});
	xpc_connection_resume(listener);
	return NULL;
}

void *lsl_helper_next(char **op, long long *mb) {
	dispatch_semaphore_wait(waiting, DISPATCH_TIME_FOREVER);
	pthread_mutex_lock(&lock);
	request *r = first;
	first = r->next;
	if (first == NULL) {
		last = NULL;
	}
	pthread_mutex_unlock(&lock);
	const char *o = xpc_dictionary_get_string(r->message, "op");
	*op = strdup(o != NULL ? o : "");
	*mb = xpc_dictionary_get_int64(r->message, "mb");
	return r;
}

void lsl_helper_reply(void *handle, int ok, const char *error, long long uid, long long limit, long long stored) {
	request *r = handle;
	xpc_object_t reply = xpc_dictionary_create_reply(r->message);
	if (reply != NULL) {
		xpc_dictionary_set_bool(reply, "ok", ok != 0);
		xpc_dictionary_set_string(reply, "error", error != NULL ? error : "");
		xpc_dictionary_set_int64(reply, "uid", uid);
		xpc_dictionary_set_int64(reply, "limit", limit);
		xpc_dictionary_set_int64(reply, "stored", stored);
		xpc_connection_send_message(r->peer, reply);
		xpc_release(reply);
	}
	xpc_release(r->message);
	xpc_release(r->peer);
	free(r);
}

int lsl_set_wired_limit_mb(int mb) {
	return sysctlbyname("iogpu.wired_limit_mb", NULL, NULL, &mb, sizeof mb) == 0 ? 0 : errno;
}
