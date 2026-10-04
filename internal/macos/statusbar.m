// The menu-bar item, for the Go code in statusbar.go: its title, and
// letting its menu work while a dialog is open.
//
// systray can show plain text or a template image in the menu bar, and a
// template image is drawn in a single colour. We want the letters in the
// menu bar's own text colour and the arrow in a status colour, so the title
// is an attributed string: text for the letters, and an SF Symbols arrow as
// an inline image.
//
// systray creates the menu-bar item but doesn't expose it. Each menu-bar
// item lives in its own window, and this app has exactly one, so we find its
// button among the app's windows by its public class, NSStatusBarButton.

#import <objc/runtime.h>

#include "common.h"
#include "statusbar.h"

// color maps the colour codes in statusbar.go to AppKit colours. The
// label colours are dynamic: they follow the menu bar's light or dark look.
static NSColor *color(int c) {
	switch (c) {
	case 1: // grey
	case 2: // faded: the same grey, but on the letters too (tertiaryLabelColor was too faint to see)
		return NSColor.secondaryLabelColor;
	case 3:
		return NSColor.systemRedColor;
	default:
		return NSColor.labelColor; // the menu bar's normal text colour
	}
}

static NSStatusBarButton *find_button(NSView *view) {
	if ([view isKindOfClass:[NSStatusBarButton class]]) {
		return (NSStatusBarButton *)view;
	}
	for (NSView *sub in view.subviews) {
		NSStatusBarButton *b = find_button(sub);
		if (b != nil) {
			return b;
		}
	}
	return nil;
}

// arrow returns the arrow as an inline image in colour c, sized and
// vertically centred to match font, or as a plain "→" if the symbol isn't
// available.
static NSAttributedString *arrow(NSFont *font, NSColor *c) {
	NSImageSymbolConfiguration *config = [[NSImageSymbolConfiguration
		configurationWithPointSize:font.pointSize weight:NSFontWeightRegular]
		configurationByApplyingConfiguration:[NSImageSymbolConfiguration configurationWithPaletteColors:@[ c ]]];
	NSImage *image = [[NSImage imageWithSystemSymbolName:@"arrowshape.right.fill" accessibilityDescription:@"to"]
		imageWithSymbolConfiguration:config];
	if (image == nil) {
		return [[NSAttributedString alloc] initWithString:@"→"
		                                       attributes:@{NSFontAttributeName : font, NSForegroundColorAttributeName : c}];
	}
	NSTextAttachment *attachment = [[NSTextAttachment alloc] init];
	attachment.image = image;
	NSSize size = image.size;
	attachment.bounds = NSMakeRect(0, (font.capHeight - size.height) / 2, size.width, size.height);
	NSMutableAttributedString *s = [[NSMutableAttributedString alloc] initWithString:@" "];
	[s appendAttributedString:[NSAttributedString attributedStringWithAttachment:attachment]];
	[s appendAttributedString:[[NSAttributedString alloc] initWithString:@" "]];
	[s addAttribute:NSFontAttributeName value:font range:NSMakeRange(0, s.length)];
	return s;
}

int lsl_set_menu_bar_title(const char *left, const char *right, int letters_color, int arrow_color) {
	NSString *leftStr = str(left);
	NSString *rightStr = str(right);
	__block int found = 0;
	on_main(^{
		for (NSWindow *window in NSApp.windows) {
			NSStatusBarButton *button = find_button(window.contentView);
			if (button == nil) {
				continue;
			}
			NSFont *font = [NSFont menuBarFontOfSize:0];
			NSDictionary *letters = @{NSFontAttributeName : font, NSForegroundColorAttributeName : color(letters_color)};
			NSMutableAttributedString *title = [[NSMutableAttributedString alloc] initWithString:leftStr attributes:letters];
			[title appendAttributedString:arrow(font, color(arrow_color))];
			[title appendAttributedString:[[NSAttributedString alloc] initWithString:rightStr attributes:letters]];
			button.image = nil;
			button.attributedTitle = title;
			found = 1;
			break;
		}
	});
	return found;
}

// While a dialog runs modally, AppKit only sends a menu item's action if the
// item's target answers YES to worksWhenModal. systray's items all target
// its SystrayAppDelegate, a plain NSObject, which answers NO, so a click on
// any of them (Quit included) did nothing while a dialog was open. This adds
// worksWhenModal, returning YES, to that class. If a later systray defines
// it already, class_addMethod leaves that one alone.
int lsl_menu_works_when_modal(void) {
	Class delegate = NSClassFromString(@"SystrayAppDelegate");
	if (delegate == nil) {
		return 0;
	}
	char types[8]; // the method's type encoding: returns BOOL, takes self and _cmd
	snprintf(types, sizeof types, "%s@:", @encode(BOOL));
	class_addMethod(delegate, @selector(worksWhenModal), imp_implementationWithBlock(^BOOL(id me) {
		return YES;
	}), types);
	return [delegate instancesRespondToSelector:@selector(worksWhenModal)];
}
