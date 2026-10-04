// Small AppKit dialogs for the Go code in dialogs.go.
//
// Two rules apply to everything here:
//
// - AppKit may only be used on the main thread. systray's run loop owns
//   that thread and our Go code runs on others, so each function hops to
//   the main thread (on_main, in common.h) and waits there until the
//   dialog closes.
//
// - The app has no Dock icon or windows (LSUIElement), so it's never the
//   active app. Each dialog activates the app first; otherwise it would
//   open behind whatever the user was looking at.

#include "common.h"
#include "dialogs.h"

char *lsl_choose_file(const char *title, const char *start_dir, int show_hidden, int dirs) {
	NSString *titleStr = str(title);
	NSString *dirStr = str(start_dir);
	__block char *result = NULL;
	on_main(^{
		ensure_edit_menu();
		NSOpenPanel *panel = [NSOpenPanel openPanel];
		panel.message = titleStr;
		panel.canChooseFiles = YES;
		panel.canChooseDirectories = dirs != 0;
		panel.canCreateDirectories = dirs != 0;
		panel.allowsMultipleSelection = NO;
		// Binaries live in places Finder hides, such as /opt/homebrew/bin.
		panel.showsHiddenFiles = show_hidden != 0;
		// Keep symlinks as chosen. Homebrew's /opt/homebrew/bin/llama-swap
		// points into a versioned Cellar directory that disappears on the
		// next upgrade; the symlink doesn't.
		panel.resolvesAliases = NO;
		if (dirStr.length > 0) {
			panel.directoryURL = [NSURL fileURLWithPath:dirStr isDirectory:YES];
		}
		[NSApp activate];
		if (lsl_run_modal(panel) == NSModalResponseOK) {
			result = strdup(panel.URL.fileSystemRepresentation);
		}
	});
	return result;
}

// output_box makes a small scrollable box for program output under an
// alert's message: a fixed-width font, lines unwrapped (they're often
// long), scrolled to the end, and selectable so it can be copied.
static NSView *output_box(NSString *text) {
	NSScrollView *scroll = [NSTextView scrollableTextView];
	scroll.frame = NSMakeRect(0, 0, 560, 160);
	scroll.hasHorizontalScroller = YES;
	scroll.borderType = NSBezelBorder;
	NSTextView *view = scroll.documentView;
	view.editable = NO;
	view.font = [NSFont monospacedSystemFontOfSize:NSFont.smallSystemFontSize weight:NSFontWeightRegular];
	view.string = text;
	// No wrapping: the text container is as wide as the longest line.
	view.horizontallyResizable = YES;
	view.maxSize = NSMakeSize(CGFLOAT_MAX, CGFLOAT_MAX);
	view.textContainer.widthTracksTextView = NO;
	view.textContainer.containerSize = NSMakeSize(CGFLOAT_MAX, CGFLOAT_MAX);
	[view scrollRangeToVisible:NSMakeRange(text.length, 0)];
	return scroll;
}

char *lsl_save_file(const char *title, const char *dir, const char *name) {
	NSString *titleStr = str(title);
	NSString *dirStr = str(dir);
	NSString *nameStr = str(name);
	__block char *result = NULL;
	on_main(^{
		ensure_edit_menu(); // the name field takes the usual editing keys
		NSSavePanel *panel = [NSSavePanel savePanel];
		panel.message = titleStr;
		panel.nameFieldStringValue = nameStr;
		panel.canCreateDirectories = YES;
		if (dirStr.length > 0) {
			panel.directoryURL = [NSURL fileURLWithPath:dirStr isDirectory:YES];
		}
		[NSApp activate];
		// A place the user picks here may be written without macOS asking
		// the app for access to that folder, Desktop included.
		if (lsl_run_modal(panel) == NSModalResponseOK) {
			result = strdup(panel.URL.fileSystemRepresentation);
		}
	});
	return result;
}

void lsl_alert(const char *message, const char *detail, const char *output) {
	NSString *messageStr = str(message);
	NSString *detailStr = str(detail);
	NSString *outputStr = str(output);
	on_main(^{
		ensure_edit_menu(); // for copying from the output box
		NSAlert *alert = [[NSAlert alloc] init];
		alert.messageText = messageStr;
		alert.informativeText = detailStr;
		if (outputStr.length > 0) {
			alert.accessoryView = output_box(outputStr);
		}
		[NSApp activate];
		lsl_run_modal(alert);
	});
}

int lsl_confirm(const char *message, const char *detail, const char *ok_title, const char *cancel_title) {
	NSString *messageStr = str(message);
	NSString *detailStr = str(detail);
	NSString *okStr = str(ok_title);
	NSString *cancelStr = str(cancel_title);
	__block NSModalResponse response = NSAlertSecondButtonReturn;
	on_main(^{
		ensure_edit_menu();
		NSAlert *alert = [[NSAlert alloc] init];
		alert.messageText = messageStr;
		alert.informativeText = detailStr;
		[alert addButtonWithTitle:okStr];     // first button: the default, answers Return
		[alert addButtonWithTitle:cancelStr]; // second: answers Escape
		[NSApp activate];
		response = lsl_run_modal(alert);
	});
	return response == NSAlertFirstButtonReturn;
}

// open_modals counts our dialogs and panels currently open, nested ones
// included, and quitting is set by lsl_close_dialogs. Both are only touched
// on the main thread.
static int open_modals = 0;
static BOOL quitting = NO;

NSModalResponse lsl_run_modal(id modal) {
	if (quitting) {
		return NSModalResponseAbort; // as if cancelled, without showing it
	}
	open_modals++;
	NSModalResponse r = [modal runModal];
	open_modals--;
	return r;
}

// close_modals closes the innermost dialog, then checks back every 10 ms,
// closing the next one out as each goes, until none is open; then it
// signals done. A dialog's runModal only returns once control gets back to
// it (after a menu that's still tracking closes, say), so the one already
// asked to close (`closing`) is left alone until it has.
static void close_modals(NSWindow *closing, dispatch_semaphore_t done) {
	if (open_modals == 0) {
		dispatch_semaphore_signal(done);
		return;
	}
	NSWindow *w = NSApp.modalWindow;
	if (w != nil && w != closing) {
		if ([w isKindOfClass:[NSSavePanel class]]) {
			// A file panel is drawn by a separate process, which ending the
			// modal session leaves on screen; its own Cancel closes it.
			[(NSSavePanel *)w cancel:nil];
		} else {
			[NSApp abortModal];
		}
		closing = w;
	}
	NSTimer *again = [NSTimer timerWithTimeInterval:0.01 repeats:NO block:^(NSTimer *t) {
		close_modals(closing, done);
	}];
	[NSRunLoop.mainRunLoop addTimer:again forMode:NSRunLoopCommonModes];
}

int lsl_close_dialogs(double timeout) {
	dispatch_semaphore_t done = dispatch_semaphore_create(0);
	main_async(^{
		quitting = YES;
		close_modals(nil, done);
	});
	return dispatch_semaphore_wait(done, dispatch_time(DISPATCH_TIME_NOW, (int64_t)(timeout * NSEC_PER_SEC))) == 0;
}
