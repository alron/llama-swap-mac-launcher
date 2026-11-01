// Copying secrets to the clipboard; see clipboard.m. lsl_copy_secret, for
// the Objective-C side, is declared in common.h.

// lsl_clear_copied empties the clipboard now if it still holds a secret the
// app copied, as at Quit.
void lsl_clear_copied(void);

// lsl_use_pasteboard makes the app copy to the pasteboard of that name
// rather than the clipboard (NULL: the clipboard again). Only for test
// programs, which must never touch the real clipboard.
void lsl_use_pasteboard(const char *name);
