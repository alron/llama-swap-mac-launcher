// Helpers shared by the Objective-C files in this package.

#import <AppKit/AppKit.h>

// main_async runs block on the main thread soon, and returns at once.
//
// It goes through the main run loop rather than GCD's main queue, because
// the main queue runs one block at a time: while a dialog shown from one
// main-queue block is open, a second block waits until it closes. The run
// loop's common modes include the one dialogs run in, so this block runs
// while one is open, like systray's own updates.
static inline void main_async(dispatch_block_t block) {
	CFRunLoopPerformBlock(CFRunLoopGetMain(), kCFRunLoopCommonModes, block);
	CFRunLoopWakeUp(CFRunLoopGetMain());
}

// on_main runs block on the main thread and waits for it to finish. AppKit
// may only be used there, and systray's run loop owns it while our Go code
// runs on other threads. It runs the block directly if we're already on the
// main thread, because waiting on the main thread for itself would deadlock.
// A dialog shown by one call doesn't hold up the others; they run while it's
// open (see main_async).
static inline void on_main(dispatch_block_t block) {
	if ([NSThread isMainThread]) {
		block();
		return;
	}
	dispatch_semaphore_t done = dispatch_semaphore_create(0);
	main_async(^{
		block();
		dispatch_semaphore_signal(done);
	});
	dispatch_semaphore_wait(done, DISPATCH_TIME_FOREVER);
}

// ensure_edit_menu gives the app a main menu with a standard Edit menu, once.
// Text fields get ⌘X, ⌘C, ⌘V, ⌘A and ⌘Z through the Edit menu: macOS matches
// the key to a menu item, which sends the field cut:, paste: and so on. A
// menu-bar-only app has no main menu, so those keys just beep. The menu is
// never shown (the app has no menu bar of its own); it only has to exist.
// Call it on the main thread before showing anything with a text field.
static inline void ensure_edit_menu(void) {
	if (NSApp.mainMenu != nil) {
		return;
	}
	NSMenu *edit = [[NSMenu alloc] initWithTitle:@"Edit"];
	[edit addItemWithTitle:@"Undo" action:@selector(undo:) keyEquivalent:@"z"];
	NSMenuItem *redo = [edit addItemWithTitle:@"Redo" action:@selector(redo:) keyEquivalent:@"z"];
	redo.keyEquivalentModifierMask = NSEventModifierFlagCommand | NSEventModifierFlagShift;
	[edit addItem:NSMenuItem.separatorItem];
	[edit addItemWithTitle:@"Cut" action:@selector(cut:) keyEquivalent:@"x"];
	[edit addItemWithTitle:@"Copy" action:@selector(copy:) keyEquivalent:@"c"];
	[edit addItemWithTitle:@"Paste" action:@selector(paste:) keyEquivalent:@"v"];
	[edit addItemWithTitle:@"Select All" action:@selector(selectAll:) keyEquivalent:@"a"];

	NSMenu *main = [[NSMenu alloc] init];
	[main addItem:[[NSMenuItem alloc] init]]; // the first item is always the app menu
	NSMenuItem *editItem = [[NSMenuItem alloc] init];
	editItem.submenu = edit;
	[main addItem:editItem];
	NSApp.mainMenu = main;
}

// lsl_run_modal runs an NSAlert or file panel modally and returns its
// response, keeping count of how many are open so lsl_close_dialogs closes
// exactly those (see dialogs.m). Every dialog here goes through it.
NSModalResponse lsl_run_modal(id modal);

// str converts a C string from Go, which may be NULL, to an NSString. It
// never returns nil: AppKit raises an exception for a nil title or text.
// Go passes only valid UTF-8 (see cString in dialogs.go), so the fallback,
// decoding byte for byte, is a backstop.
static inline NSString *str(const char *s) {
	if (s == NULL) {
		return @"";
	}
	NSString *v = [NSString stringWithUTF8String:s];
	return v ?: [NSString stringWithCString:s encoding:NSISOLatin1StringEncoding];
}
