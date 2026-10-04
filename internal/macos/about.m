// The About panel, for the Go code in about.go.
//
// macOS has a standard About panel: the app's icon, name and version, a
// credits area, and the copyright line from Info.plist
// (NSHumanReadableCopyright). It's an ordinary window, not a dialog: it
// stays open until it's closed and blocks nothing, so it doesn't go
// through lsl_run_modal.

#include "common.h"
#include "about.h"

void lsl_show_about(const char *version, const char *credits, const char *link) {
	NSString *versionStr = str(version);
	NSString *creditsStr = str(credits);
	NSString *linkStr = str(link);
	on_main(^{
		ensure_edit_menu(); // the credits can be selected and copied
		NSMutableParagraphStyle *centred = [[NSMutableParagraphStyle alloc] init];
		centred.alignment = NSTextAlignmentCenter;
		NSDictionary *attrs = @{
			NSFontAttributeName : [NSFont systemFontOfSize:NSFont.smallSystemFontSize],
			NSForegroundColorAttributeName : NSColor.secondaryLabelColor,
			NSParagraphStyleAttributeName : centred,
		};
		NSMutableAttributedString *text = [[NSMutableAttributedString alloc] initWithString:creditsStr attributes:attrs];
		NSURL *url = [NSURL URLWithString:linkStr];
		if (linkStr.length > 0 && url != nil) {
			// Shown without "https://"; a click opens it in the browser.
			NSString *shown = url.host ? [url.host stringByAppendingString:url.path] : linkStr;
			NSMutableDictionary *linkAttrs = [attrs mutableCopy];
			linkAttrs[NSLinkAttributeName] = url;
			[text appendAttributedString:[[NSAttributedString alloc] initWithString:@"\n" attributes:attrs]];
			[text appendAttributedString:[[NSAttributedString alloc] initWithString:shown attributes:linkAttrs]];
		}
		// Ask to become the active app, so the panel opens in front and
		// takes the keyboard. Since macOS 14 that's only a request, which
		// macOS sometimes refuses; the panel then opened behind the active
		// app's windows (seen 2026-10-04, intermittently). Dialogs don't
		// mind, since they sit above ordinary windows; the About panel is
		// one, so it's brought to the front below regardless.
		[NSApp activate];
		[NSApp orderFrontStandardAboutPanelWithOptions:@{
			NSAboutPanelOptionApplicationVersion : versionStr, // shown as "Version …"
			// The build number, shown after the version in brackets. It
			// would default to CFBundleVersion, the same as the version
			// ("Version 0.1.3 (0.1.3)"); empty, only the version shows.
			NSAboutPanelOptionVersion : @"",
			NSAboutPanelOptionCredits : text,
		}];
		// AppKit doesn't hand out the panel, but it's this app's only
		// ordinary window (the menu bar item and dialogs sit at higher
		// levels), and orderFrontRegardless brings a window to the front
		// even when its app isn't active.
		for (NSWindow *w in NSApp.windows) {
			if (w.isVisible && w.level == NSNormalWindowLevel) {
				[w orderFrontRegardless];
			}
		}
	});
}
