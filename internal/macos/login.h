// C interface to the Objective-C in login.m. See login.go.

// lsl_login_item_status returns the app's SMAppServiceStatus as a login
// item: 0 not registered, 1 enabled, 2 requires approval, 3 not found.
int lsl_login_item_status(void);

// lsl_login_item_set registers (enable != 0) or unregisters the app as a
// login item. It returns NULL on success, or an error message the caller
// must free().
char *lsl_login_item_set(int enable);

// lsl_open_login_items_settings opens System Settings at Login Items.
void lsl_open_login_items_settings(void);
