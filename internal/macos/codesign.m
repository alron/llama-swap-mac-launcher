// Checking that a newer copy of the app installed over the running one is
// really ours before restarting into it (TODO item 65), for the Go code in
// codesign.go. Security framework calls, safe from any thread.

#import <Foundation/Foundation.h>
#import <Security/Security.h>

#include "codesign.h"

// The running app's designated requirement: for a Developer ID build, its
// bundle ID and the team that signed it. Read from the running code, so it
// stays known after the bundle on disk has been replaced.
static SecRequirementRef selfRequirement;

static char *describe(OSStatus status, CFErrorRef error) {
	NSString *why = error != NULL ? ((__bridge NSError *)error).localizedDescription
	                              : [NSString stringWithFormat:@"OSStatus %d", (int)status];
	return strdup(why.UTF8String);
}

char *lsl_capture_self_requirement(void) {
	SecCodeRef self = NULL;
	OSStatus status = SecCodeCopySelf(kSecCSDefaultFlags, &self);
	if (status != errSecSuccess) {
		return describe(status, NULL);
	}
	status = SecCodeCopyDesignatedRequirement(self, kSecCSDefaultFlags, &selfRequirement);
	CFRelease(self);
	return status == errSecSuccess ? NULL : describe(status, NULL);
}

char *lsl_matches_self(const char *path) {
	if (selfRequirement == NULL) {
		return strdup("the app's own signing requirement wasn't read at launch");
	}
	NSURL *url = [NSURL fileURLWithPath:[NSString stringWithUTF8String:path]];
	SecStaticCodeRef code = NULL;
	OSStatus status = SecStaticCodeCreateWithPath((__bridge CFURLRef)url, kSecCSDefaultFlags, &code);
	if (status != errSecSuccess) {
		return describe(status, NULL);
	}
	// Strict, and every nested binary too (llsl): a half-copied or altered
	// bundle fails, as does one signed by anyone else.
	CFErrorRef error = NULL;
	status = SecStaticCodeCheckValidityWithErrors(code, kSecCSStrictValidate | kSecCSCheckAllArchitectures | kSecCSCheckNestedCode,
	                                              selfRequirement, &error);
	CFRelease(code);
	char *why = status == errSecSuccess ? NULL : describe(status, error);
	if (error != NULL) {
		CFRelease(error);
	}
	return why;
}
