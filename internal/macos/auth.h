// C interface to the Objective-C in auth.m. See auth.go.

// lsl_confirm_owner asks the person at the Mac to confirm, with Touch ID, a
// paired Apple Watch or their login password, and waits for the answer: 0
// confirmed, 1 cancelled, 2 failed or unavailable, with *error set (for
// free()) to why.
int lsl_confirm_owner(const char *reason, char **error);

// lsl_cancel_confirm_owner cancels a confirmation in progress, at Quit.
void lsl_cancel_confirm_owner(void);
