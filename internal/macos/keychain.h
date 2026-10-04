// C interface to the Objective-C in keychain.m. See keychain.go.

// Each returns an OSStatus: 0 for success, errSecItemNotFound (-25300)
// when there's no such item, or another Security framework error.

// lsl_keychain_set stores secret under service and account, replacing
// any value already there. label is what Keychain Access shows.
int lsl_keychain_set(const char *service, const char *account, const char *label, const char *secret);

// lsl_keychain_get sets *secret to the stored value, which the caller
// must free().
int lsl_keychain_get(const char *service, const char *account, char **secret);

// lsl_keychain_delete removes the item, if there is one.
int lsl_keychain_delete(const char *service, const char *account);

// lsl_keychain_message returns a description of status, which the caller
// must free().
char *lsl_keychain_message(int status);
