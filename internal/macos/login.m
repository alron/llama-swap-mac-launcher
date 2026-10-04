// Launch at login, through SMAppService (macOS 13+), for the Go code in
// login.go. mainAppService is the app itself; macOS lists it under System
// Settings → General → Login Items, where the user can also turn it off.
// SMAppService requires the app to be code signed.

#import <ServiceManagement/ServiceManagement.h>
#include "login.h"

int lsl_login_item_status(void) {
	return (int)SMAppService.mainAppService.status;
}

char *lsl_login_item_set(int enable) {
	NSError *error = nil;
	BOOL ok = enable ? [SMAppService.mainAppService registerAndReturnError:&error]
	                 : [SMAppService.mainAppService unregisterAndReturnError:&error];
	if (ok) {
		return NULL;
	}
	const char *msg = error.localizedDescription.UTF8String;
	return strdup(msg ? msg : "unknown error");
}

void lsl_open_login_items_settings(void) {
	[SMAppService openSystemSettingsLoginItems];
}
