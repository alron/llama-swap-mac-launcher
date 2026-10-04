// The login keychain, for the Go code in keychain.go: secrets the app
// keeps out of its preferences file, such as llama-swap's API key.
//
// Each secret is a "generic password" item, found by its service (the
// app's bundle ID, so dev and release builds keep separate items) and its
// account (which secret it is). The Security framework calls here don't
// touch the UI, so unlike the other shims they run on any thread.

#import <Security/Security.h>

#include "common.h"
#include "keychain.h"

// query finds the item for service and account.
static NSMutableDictionary *query(const char *service, const char *account) {
	return [@{
		(id)kSecClass : (id)kSecClassGenericPassword,
		(id)kSecAttrService : str(service),
		(id)kSecAttrAccount : str(account),
	} mutableCopy];
}

int lsl_keychain_set(const char *service, const char *account, const char *label, const char *secret) {
	NSData *data = [str(secret) dataUsingEncoding:NSUTF8StringEncoding];
	NSMutableDictionary *q = query(service, account);
	// Replace the value if the item exists; otherwise add it.
	OSStatus status = SecItemUpdate((CFDictionaryRef)q, (CFDictionaryRef) @{(id)kSecValueData : data});
	if (status == errSecItemNotFound) {
		q[(id)kSecAttrLabel] = str(label);
		q[(id)kSecValueData] = data;
		status = SecItemAdd((CFDictionaryRef)q, NULL);
	}
	return status;
}

int lsl_keychain_get(const char *service, const char *account, char **secret) {
	NSMutableDictionary *q = query(service, account);
	q[(id)kSecReturnData] = @YES;
	q[(id)kSecMatchLimit] = (id)kSecMatchLimitOne;
	CFTypeRef result = NULL;
	OSStatus status = SecItemCopyMatching((CFDictionaryRef)q, &result);
	if (status == errSecSuccess) {
		NSData *data = (__bridge_transfer NSData *)result;
		NSString *s = [[NSString alloc] initWithData:data encoding:NSUTF8StringEncoding];
		*secret = strdup(s ? s.UTF8String : "");
	}
	return status;
}

int lsl_keychain_delete(const char *service, const char *account) {
	OSStatus status = SecItemDelete((CFDictionaryRef)query(service, account));
	return status == errSecItemNotFound ? errSecSuccess : status;
}

char *lsl_keychain_message(int status) {
	NSString *msg = (__bridge_transfer NSString *)SecCopyErrorMessageString(status, NULL);
	return strdup(msg ? msg.UTF8String : "unknown keychain error");
}
