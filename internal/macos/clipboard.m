// Copying a secret to the clipboard, for the Copy buttons on the Secrets
// tab (TODO item 54), with the protections password managers use:
//
// - only on this Mac: Universal Clipboard doesn't send it to the user's
//   other devices;
// - marked concealed and transient (nspasteboard.org's conventions), so
//   clipboard managers that follow them don't record it;
// - cleared after a while, and when the app quits, but only if nothing has
//   been copied since. The clipboard's change count goes up with every
//   write, by anyone, and can be read without reading what's there, so the
//   app never reads the clipboard, and leaves alone whatever was copied
//   after it. With the Tweaks tab's Never, it isn't cleared at all.

#include "common.h"
#include "clipboard.h"

static NSPasteboard *board;         // where secrets go: the clipboard, or a test program's own
static NSInteger copied = -1;       // board's change count after the app's copy, or -1
static NSTimer *clearTimer;         // clears board when the time is up

static NSPasteboard *pasteboard(void) {
	return board ?: NSPasteboard.generalPasteboard;
}

// clear empties the clipboard if it still holds the app's copy. Main thread.
static void clear(void) {
	[clearTimer invalidate];
	clearTimer = nil;
	if (copied >= 0 && pasteboard().changeCount == copied) {
		[pasteboard() clearContents];
	}
	copied = -1;
}

void lsl_copy_secret(NSString *value, NSTimeInterval seconds) {
	NSPasteboard *pb = pasteboard();
	[pb prepareForNewContentsWithOptions:NSPasteboardContentsCurrentHostOnly];
	NSPasteboardItem *item = [[NSPasteboardItem alloc] init];
	[item setString:value forType:NSPasteboardTypeString];
	// The markers' presence is what counts; they carry nothing.
	[item setData:[NSData data] forType:@"org.nspasteboard.ConcealedType"];
	[item setData:[NSData data] forType:@"org.nspasteboard.TransientType"];
	if (![pb writeObjects:@[ item ]]) {
		NSBeep();
		return;
	}
	[clearTimer invalidate];
	clearTimer = nil;
	if (seconds <= 0) {
		copied = -1; // never: left on the clipboard, Quit included
		return;
	}
	copied = pb.changeCount;
	// In the common modes, so it fires while a dialog is open too.
	clearTimer = [NSTimer timerWithTimeInterval:seconds repeats:NO block:^(NSTimer *t) { clear(); }];
	[NSRunLoop.mainRunLoop addTimer:clearTimer forMode:NSRunLoopCommonModes];
}

void lsl_clear_copied(void) {
	on_main(^{ clear(); });
}

void lsl_use_pasteboard(const char *name) {
	on_main(^{ board = name ? [NSPasteboard pasteboardWithName:str(name)] : nil; });
}
