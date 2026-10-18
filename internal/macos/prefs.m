// The Preferences dialog, for the Go code in prefs.go.
//
// It's an alert (Save, Cancel) with tabs as its accessory view: General, a
// grid of labels and fields; Secrets, two tables of named secrets (API keys
// and secret environment variables); Peers; and Logs. The values go in and come back
// out as JSON, so the C interface is a single function and all the
// checking stays in Go. If what the user entered doesn't check out, Go
// shows the dialog again with their entries and the error.

#import <Security/Security.h>

#include "common.h"
#include "dialogs.h"
#include "prefs.h"

// LSLSecretTable runs one table of named secrets and its buttons: the API
// keys, or the secret environment variables. AppKit asks an object for a
// table's rows and cells, and reports edits and clicks to it, so each table
// has one.
@interface LSLSecretTable : NSObject <NSTableViewDataSource, NSTableViewDelegate>
// The rows: each a dictionary with "name" and "value".
@property(strong) NSMutableArray<NSMutableDictionary *> *rows;
@property(strong) NSString *prefix; // shown before each name: LLSL_ for keys, none for variables
@property BOOL capitals;            // names are turned into capitals (keys)
@property BOOL generates;           // + fills in a new random key, and Generate makes another (keys)
@property(copy) NSString *placeholder;
@property(copy) void (^namesChanged)(void); // after a name changes, or a row comes or goes
@property(strong) NSTableView *table;
@property(strong) NSButton *show;
@property NSInteger revealed; // the row whose value is shown, or -1
@end

@implementation LSLSecretTable
- (instancetype)initWithRows:(id)rows {
	self = [super init];
	self.rows = [NSMutableArray array];
	if ([rows isKindOfClass:[NSArray class]]) {
		for (id r in rows) {
			if ([r isKindOfClass:[NSDictionary class]]) {
				id name = r[@"name"], value = r[@"value"];
				[self.rows addObject:[@{
					@"name" : [name isKindOfClass:[NSString class]] ? name : @"",
					@"value" : [value isKindOfClass:[NSString class]] ? value : @"",
				} mutableCopy]];
			}
		}
	}
	self.prefix = @"";
	self.revealed = -1;
	return self;
}

- (NSInteger)numberOfRowsInTableView:(NSTableView *)tableView {
	return (NSInteger)self.rows.count;
}

- (NSView *)tableView:(NSTableView *)tableView viewForTableColumn:(NSTableColumn *)column row:(NSInteger)row {
	NSMutableDictionary *r = self.rows[(NSUInteger)row];
	if ([column.identifier isEqual:@"name"]) {
		NSTextField *name = [self cellField:r[@"name"] secure:NO action:@selector(nameEdited:)];
		name.placeholderString = self.placeholder;
		if (self.prefix.length == 0) {
			return name;
		}
		// The prefix, fixed, then the editable rest of the name.
		NSTextField *prefix = [NSTextField labelWithString:self.prefix];
		prefix.textColor = NSColor.secondaryLabelColor;
		NSStackView *s = [NSStackView stackViewWithViews:@[ prefix, name ]];
		s.spacing = 0;
		return s;
	}
	// The value: dots, unless it's the row Show revealed.
	NSTextField *value = [self cellField:r[@"value"] secure:(row != self.revealed) action:@selector(valueEdited:)];
	value.font = [NSFont monospacedSystemFontOfSize:NSFont.smallSystemFontSize weight:NSFontWeightRegular];
	return value;
}

// cellField is an editable field without a border, for a table cell. Its
// action reports the edit when editing ends, not only on Return.
- (NSTextField *)cellField:(NSString *)value secure:(BOOL)secure action:(SEL)action {
	NSTextField *f = secure ? [[NSSecureTextField alloc] init] : [[NSTextField alloc] init];
	f.stringValue = value ?: @"";
	f.bordered = NO;
	f.drawsBackground = NO;
	f.editable = YES;
	f.target = self;
	f.action = action;
	f.cell.sendsActionOnEndEditing = YES;
	f.cell.scrollable = YES;
	return f;
}

- (void)nameEdited:(NSTextField *)sender {
	NSInteger row = [self.table rowForView:sender];
	if (row < 0 || row >= (NSInteger)self.rows.count) {
		return;
	}
	NSString *name = [sender.stringValue stringByTrimmingCharactersInSet:NSCharacterSet.whitespaceAndNewlineCharacterSet];
	if (self.capitals) {
		name = name.uppercaseString; // key names are capitals; Go checks the rest
	}
	sender.stringValue = name;
	self.rows[(NSUInteger)row][@"name"] = name;
	[self changed];
}

- (void)valueEdited:(NSTextField *)sender {
	NSInteger row = [self.table rowForView:sender];
	if (row >= 0 && row < (NSInteger)self.rows.count) {
		self.rows[(NSUInteger)row][@"value"] = sender.stringValue;
	}
}

- (void)changed {
	if (self.namesChanged) {
		self.namesChanged();
	}
}

// Moving to another row hides a revealed value again.
- (void)tableViewSelectionDidChange:(NSNotification *)notification {
	[self reveal:-1];
}

// reveal shows row's value in plain text (-1 for none), and hides any other.
- (void)reveal:(NSInteger)row {
	NSInteger before = self.revealed;
	self.revealed = row;
	NSMutableIndexSet *rows = [NSMutableIndexSet indexSet];
	for (NSNumber *r in @[ @(before), @(row) ]) {
		if (r.integerValue >= 0 && r.integerValue < (NSInteger)self.rows.count) {
			[rows addIndex:(NSUInteger)r.integerValue];
		}
	}
	[self.table reloadDataForRowIndexes:rows columnIndexes:[NSIndexSet indexSetWithIndex:1]];
	self.show.title = row >= 0 ? @"Hide" : @"Show";
}

- (void)showValue:(id)sender {
	NSInteger row = self.table.selectedRow;
	[self reveal:(self.revealed >= 0 || row < 0) ? -1 : row];
}

// newKey is a random key, in the form llama-swap's example config suggests:
// "sk-" and 48 random bytes in base64.
static NSString *newKey(void) {
	uint8_t bytes[48];
	if (SecRandomCopyBytes(kSecRandomDefault, sizeof bytes, bytes) != errSecSuccess) {
		return @"";
	}
	NSString *b64 = [[NSData dataWithBytes:bytes length:sizeof bytes] base64EncodedStringWithOptions:0];
	return [@"sk-" stringByAppendingString:b64];
}

// + adds a row (with a new random key, for keys) and starts editing its
// name.
- (void)addRow:(id)sender {
	[self.table.window makeFirstResponder:nil]; // finish any edit first
	[self.rows addObject:[@{@"name" : @"", @"value" : self.generates ? newKey() : @""} mutableCopy]];
	NSInteger row = (NSInteger)self.rows.count - 1;
	[self.table reloadData];
	[self.table selectRowIndexes:[NSIndexSet indexSetWithIndex:(NSUInteger)row] byExtendingSelection:NO];
	[self.table editColumn:0 row:row withEvent:nil select:YES];
	[self changed];
}

- (void)removeRow:(id)sender {
	NSInteger row = self.table.selectedRow;
	if (row < 0) {
		return;
	}
	[self.table.window makeFirstResponder:nil];
	[self.rows removeObjectAtIndex:(NSUInteger)row];
	self.revealed = -1;
	self.show.title = @"Show";
	[self.table reloadData];
	[self changed];
}

// Generate replaces the selected key with a new random one.
- (void)generate:(id)sender {
	NSInteger row = self.table.selectedRow;
	if (row < 0) {
		return;
	}
	[self.table.window makeFirstResponder:nil];
	self.rows[(NSUInteger)row][@"value"] = newKey();
	[self.table reloadDataForRowIndexes:[NSIndexSet indexSetWithIndex:(NSUInteger)row]
	                      columnIndexes:[NSIndexSet indexSetWithIndex:1]];
}

// json is the rows, for the dialog's reply.
- (NSArray *)json {
	NSMutableArray *out = [NSMutableArray array];
	for (NSDictionary *r in self.rows) {
		[out addObject:@{@"name" : r[@"name"], @"value" : r[@"value"]}];
	}
	return out;
}
@end

// LSLPeers runs the Peers tab: a table of peers (name and URL), and for the
// selected one, its key (one of the secret variables) and a checklist of
// its models. A provider can serve hundreds of models, so the checklist is a
// table, which only draws the rows on screen, with a filter, and buttons
// that act on the rows the filter shows. Fetch models asks the app's Go
// code, off the main thread, which asks the peer.
@interface LSLPeers : NSObject <NSTableViewDataSource, NSTableViewDelegate, NSSearchFieldDelegate>
// Each peer: "name", "url", "key" (a secret variable's name, or "") and
// "models", each model a dictionary with "id" and "on" (ticked).
@property(strong) NSMutableArray<NSMutableDictionary *> *peers;
@property(strong) NSMutableArray<NSMutableDictionary *> *shown; // the selected peer's models the filter shows
@property(strong) NSTableView *table;                           // the peers
@property(strong) NSTableView *list;                            // the selected peer's models
@property(strong) NSSearchField *filter;
@property(strong) NSPopUpButton *key;
@property(strong) NSButton *fetch;
@property(strong) NSProgressIndicator *spinner;
@property(strong) NSTextField *status;
@property(strong) NSTextField *count;
@property(strong) NSTextField *addField;
@property(strong) NSArray<NSControl *> *detail; // usable while a peer is selected
@property(strong) NSArray<NSControl *> *all;    // usable with the managed folder
@property(strong) NSTextField *managedHint;
@property(weak) LSLSecretTable *vars; // the secret variables: the key menu, and the key's value for Fetch
@property(nonatomic) BOOL managed;
- (void)showSelected;
- (void)refreshKeys;
- (NSArray *)json;
@end

// LSLPrefsController runs the rest of the dialog that calls back: the
// Choose… buttons, Tab in the multi-line fields, and the popup choosing the
// key the app uses.
@interface LSLPrefsController : NSObject <NSTextViewDelegate>
@property(strong) NSTextField *binary;
@property(strong) NSTextField *config;
@property(strong) NSTextField *launcherLog;
@property(strong) NSTextField *ownLog; // llama-swap's own log
@property(strong) NSButton *chooseOwnLog;
@property(strong) NSPopUpButton *logMode;
@property(strong) LSLSecretTable *keys;
@property(strong) LSLSecretTable *vars;
@property(strong) NSPopUpButton *appKey;
@property(strong) LSLPeers *peers; // the Peers tab
// The config folder: the menu, the field, and the user's own folder, kept
// while the field shows the app's managed one.
@property(strong) NSPopUpButton *dirMode;
@property(strong) NSTextField *configDir;
@property(strong) NSButton *chooseDir;
@property(strong) NSButton *showDir;
@property(copy) NSString *ownDir;
@property(copy) NSString *managedDir;
@property(copy) NSString *lastMode;
// Require these keys, which needs the managed folder, and the hint under it.
@property(strong) NSButton *manageKeys;
@property(strong) NSTextField *manageKeysHint;
@property(copy) NSString *keysExample;
@end

@implementation LSLPrefsController
- (void)chooseBinary:(id)sender {
	[self choose:@"Choose the llama-swap binary" into:self.binary startingIn:@"/opt/homebrew/bin"];
}
- (void)chooseConfig:(id)sender {
	[self choose:@"Choose the llama-swap config file" into:self.config startingIn:NSHomeDirectory()];
}
- (void)chooseLauncherLog:(id)sender {
	[self chooseLog:@"Choose the launcher log, or a folder to put it in" into:self.launcherLog];
}
- (void)chooseOwnLog:(id)sender {
	[self chooseLog:@"Choose llama-swap's log, or a folder to put it in" into:self.ownLog];
}
- (void)chooseConfigDir:(id)sender {
	NSString *start = self.configDir.stringValue.length > 0 ? self.configDir.stringValue.stringByExpandingTildeInPath : NSHomeDirectory();
	char *path = lsl_choose_file("Choose the folder of config files for llama-swap", start.UTF8String, 0, 2);
	if (path != NULL) {
		self.configDir.stringValue = [NSString stringWithUTF8String:path].stringByAbbreviatingWithTildeInPath;
		free(path);
	}
}
// showConfigDir opens the folder in the Finder. The managed one may not
// exist until llama-swap first starts with it, so it's made here if need be,
// readable by the user alone, as the app makes it.
- (void)showConfigDir:(id)sender {
	NSString *dir = self.configDir.stringValue.stringByExpandingTildeInPath;
	if (dir.length == 0) {
		return;
	}
	if ([dir isEqual:self.managedDir]) {
		[NSFileManager.defaultManager createDirectoryAtPath:dir withIntermediateDirectories:YES
		                                         attributes:@{NSFilePosixPermissions : @0700} error:nil];
	}
	[NSWorkspace.sharedWorkspace openURL:[NSURL fileURLWithPath:dir isDirectory:YES]];
}
// dirModeChanged sets up the field for the folder chosen in the menu: the
// user's own, editable; the app's, greyed out; or none. Require these keys,
// on the Secrets tab, works only with the app's folder.
- (void)dirModeChanged:(id)sender {
	NSString *mode = self.dirMode.selectedItem.representedObject;
	if ([self.lastMode isEqual:@"own"]) {
		self.ownDir = self.configDir.stringValue; // kept for switching back
	}
	self.lastMode = mode;
	BOOL own = [mode isEqual:@"own"], managed = [mode isEqual:@"managed"];
	self.configDir.stringValue = own ? self.ownDir : managed ? self.managedDir.stringByAbbreviatingWithTildeInPath : @"";
	self.configDir.enabled = own;
	self.configDir.toolTip = managed ? self.managedDir : nil; // the field may cut it short
	self.chooseDir.enabled = own;
	self.showDir.enabled = own || managed;
	self.manageKeys.enabled = managed;
	[self.peers setManaged:managed]; // peers, too, are written to the managed folder
	self.manageKeysHint.stringValue = managed
		? [NSString stringWithFormat:@"The app lists them in llama-swap's apiKeys, in its managed config folder. "
		                             @"Otherwise, list them in llama-swap's config yourself:\n%@", self.keysExample]
		: [NSString stringWithFormat:@"Needs Config folder: Managed by the app, on the General tab. "
		                             @"Until then, list them in llama-swap's config yourself:\n%@", self.keysExample];
}
// ownDirValue is the user's own folder, whichever folder the field shows.
- (NSString *)ownDirValue {
	return [self.lastMode isEqual:@"own"] ? self.configDir.stringValue : self.ownDir;
}
// choose opens a file panel (from dialogs.m) over the Preferences dialog,
// starting where the field's current path is, and puts the choice in field.
- (void)choose:(NSString *)title into:(NSTextField *)field startingIn:(NSString *)fallback {
	NSString *dir = field.stringValue.length > 0 ? field.stringValue.stringByExpandingTildeInPath.stringByDeletingLastPathComponent : fallback;
	char *path = lsl_choose_file(title.UTF8String, dir.UTF8String, 1, 0);
	if (path != NULL) {
		field.stringValue = [NSString stringWithUTF8String:path];
		free(path);
	}
}
// chooseLog takes an existing file, or a folder, where the log gets its
// default name: a log may not exist yet, and a Save panel would ask to
// replace one that does, though the app appends to it.
- (void)chooseLog:(NSString *)title into:(NSTextField *)field {
	NSString *current = field.stringValue.length > 0 ? field.stringValue : field.placeholderString;
	char *path = lsl_choose_file(title.UTF8String, current.stringByExpandingTildeInPath.stringByDeletingLastPathComponent.UTF8String, 1, 1);
	if (path == NULL) {
		return;
	}
	NSString *chosen = [NSString stringWithUTF8String:path];
	free(path);
	BOOL folder = NO;
	if ([NSFileManager.defaultManager fileExistsAtPath:chosen isDirectory:&folder] && folder) {
		chosen = [chosen stringByAppendingPathComponent:field.placeholderString.lastPathComponent];
	}
	field.stringValue = chosen.stringByAbbreviatingWithTildeInPath;
}
// llama-swap's own log only matters when its output goes there.
- (void)logModeChanged:(id)sender {
	BOOL own = [self.logMode.selectedItem.representedObject isEqual:@"file"];
	self.ownLog.enabled = own;
	self.chooseOwnLog.enabled = own;
}
// A text view types Tab as a tab character. In a form, Tab should move to
// the next field, as it does in the one-line fields; Option-Tab still types
// a tab, as usual on macOS.
- (BOOL)textView:(NSTextView *)textView doCommandBySelector:(SEL)command {
	if (command == @selector(insertTab:)) {
		[textView.window selectNextKeyView:textView];
		return YES;
	}
	if (command == @selector(insertBacktab:)) {
		[textView.window selectPreviousKeyView:textView];
		return YES;
	}
	return NO;
}

// refreshAppKey lists the named keys in the "app uses" popup, keeping the
// choice if that key is still there.
- (void)refreshAppKey {
	NSString *chosen = self.appKey.selectedItem.representedObject;
	[self.appKey removeAllItems];
	for (NSDictionary *key in self.keys.rows) {
		NSString *name = key[@"name"];
		NSString *title = [self.keys.prefix stringByAppendingString:name];
		if (name.length == 0 || [self.appKey itemWithTitle:title] != nil) {
			continue;
		}
		[self.appKey addItemWithTitle:title];
		self.appKey.lastItem.representedObject = name;
	}
	if (chosen != nil) {
		NSInteger i = [self.appKey indexOfItemWithRepresentedObject:chosen];
		if (i >= 0) {
			[self.appKey selectItemAtIndex:i];
		}
	}
	self.appKey.enabled = self.appKey.numberOfItems > 0;
}
@end

static NSString *string(NSDictionary *d, NSString *key) {
	id v = d[key];
	return [v isKindOfClass:[NSString class]] ? v : @"";
}

static NSTextField *label(NSString *text) {
	return [NSTextField labelWithString:text];
}

// hint is the small grey text under a field. It wraps at the fields'
// width rather than widening the dialog.
static NSTextField *hint(NSString *text) {
	NSTextField *l = [NSTextField wrappingLabelWithString:text];
	l.font = [NSFont systemFontOfSize:NSFont.smallSystemFontSize];
	l.textColor = NSColor.secondaryLabelColor;
	l.preferredMaxLayoutWidth = 420;
	return l;
}

static NSTextField *field(NSString *value, NSString *placeholder, CGFloat width) {
	NSTextField *f = [NSTextField textFieldWithString:value];
	f.placeholderString = placeholder;
	[f.widthAnchor constraintEqualToConstant:width].active = YES;
	return f;
}

// area is a multi-line field, for one argument or variable per line. It
// uses a fixed-width font, and turns off smart quotes and dashes, so what's
// typed is exactly what llama-swap gets. The controller handles its Tab key.
static NSScrollView *area(NSString *value, LSLPrefsController *controller, NSTextView **text) {
	NSScrollView *scroll = [NSTextView scrollableTextView];
	scroll.borderType = NSBezelBorder;
	[scroll.widthAnchor constraintEqualToConstant:420].active = YES;
	[scroll.heightAnchor constraintEqualToConstant:64].active = YES;
	NSTextView *t = scroll.documentView;
	t.string = value;
	t.font = [NSFont userFixedPitchFontOfSize:NSFont.smallSystemFontSize];
	t.automaticQuoteSubstitutionEnabled = NO;
	t.automaticDashSubstitutionEnabled = NO;
	t.automaticTextReplacementEnabled = NO;
	t.automaticSpellingCorrectionEnabled = NO;
	t.delegate = controller;
	*text = t;
	return scroll;
}

static NSButton *checkbox(NSString *title, id value) {
	NSButton *b = [NSButton checkboxWithTitle:title target:nil action:nil];
	b.state = [value boolValue] ? NSControlStateValueOn : NSControlStateValueOff;
	return b;
}

// row lays views out side by side.
static NSStackView *row(NSArray<NSView *> *views) {
	NSStackView *s = [NSStackView stackViewWithViews:views];
	s.spacing = 8;
	return s;
}

// column stacks views, lined up on the left.
static NSStackView *column(NSArray<NSView *> *views) {
	NSStackView *s = [NSStackView stackViewWithViews:views];
	s.orientation = NSUserInterfaceLayoutOrientationVertical;
	s.alignment = NSLayoutAttributeLeading;
	s.spacing = 6;
	return s;
}

// grid lines up labels and fields in two columns, labels on the right.
static NSGridView *grid(NSArray<NSArray<NSView *> *> *rows) {
	NSGridView *g = [NSGridView gridViewWithViews:rows];
	g.rowSpacing = 6;
	g.columnSpacing = 8;
	g.rowAlignment = NSGridRowAlignmentFirstBaseline;
	[g columnAtIndex:0].xPlacement = NSGridCellPlacementTrailing;
	return g;
}

// topAligned makes a grid row's label sit at the top of a tall field.
static void topAligned(NSGridView *g, NSInteger i) {
	NSGridRow *r = [g rowAtIndex:i];
	r.rowAlignment = NSGridRowAlignmentNone;
	r.yPlacement = NSGridCellPlacementTop;
}

// smallButton is a borderless icon button, like the + and − under lists in
// System Settings.
static NSButton *smallButton(NSString *image, NSString *description, id target, SEL action) {
	NSButton *b = [NSButton buttonWithImage:[NSImage imageNamed:image] target:target action:action];
	b.bezelStyle = NSBezelStyleSmallSquare;
	b.accessibilityLabel = description;
	return b;
}

// secretTable is t's table in a scroll view, with + and −, Generate (for
// keys) and Show under it.
static NSView *secretTable(LSLSecretTable *t, NSString *nameTitle, NSString *valueTitle, CGFloat height) {
	NSTableView *table = [[NSTableView alloc] init];
	NSTableColumn *name = [[NSTableColumn alloc] initWithIdentifier:@"name"];
	name.title = nameTitle;
	name.width = 190;
	NSTableColumn *value = [[NSTableColumn alloc] initWithIdentifier:@"value"];
	value.title = valueTitle;
	value.width = 330;
	[table addTableColumn:name];
	[table addTableColumn:value];
	table.dataSource = t;
	table.delegate = t;
	table.usesAlternatingRowBackgroundColors = YES;
	table.columnAutoresizingStyle = NSTableViewLastColumnOnlyAutoresizingStyle;
	t.table = table;
	NSScrollView *scroll = [[NSScrollView alloc] init];
	scroll.documentView = table;
	scroll.hasVerticalScroller = YES;
	scroll.borderType = NSBezelBorder;
	[scroll.widthAnchor constraintEqualToConstant:540].active = YES;
	[scroll.heightAnchor constraintEqualToConstant:height].active = YES;

	t.show = [NSButton buttonWithTitle:@"Show" target:t action:@selector(showValue:)];
	NSMutableArray *buttons = [@[
		smallButton(NSImageNameAddTemplate, @"Add", t, @selector(addRow:)),
		smallButton(NSImageNameRemoveTemplate, @"Remove the selected row", t, @selector(removeRow:)),
	] mutableCopy];
	if (t.generates) {
		[buttons addObject:[NSButton buttonWithTitle:@"Generate" target:t action:@selector(generate:)]];
	}
	[buttons addObject:t.show];
	return column(@[ scroll, row(buttons) ]);
}

// padded puts view in a container with a margin, pinned to its top left,
// so a tab shows it there rather than stretched to the tab's size.
static NSView *padded(NSView *view) {
	NSView *box = [[NSView alloc] init];
	view.translatesAutoresizingMaskIntoConstraints = NO;
	[box addSubview:view];
	[NSLayoutConstraint activateConstraints:@[
		[view.leadingAnchor constraintEqualToAnchor:box.leadingAnchor constant:16],
		[view.topAnchor constraintEqualToAnchor:box.topAnchor constant:16],
		[box.trailingAnchor constraintGreaterThanOrEqualToAnchor:view.trailingAnchor constant:16],
		[box.bottomAnchor constraintGreaterThanOrEqualToAnchor:view.bottomAnchor constant:16],
	]];
	return box;
}

static NSTabViewItem *tab(NSString *identifier, NSString *title, NSView *view) {
	NSTabViewItem *item = [[NSTabViewItem alloc] initWithIdentifier:identifier];
	item.label = title;
	item.view = padded(view);
	return item;
}

// sentenceCase capitalizes a message's first letter: Go's errors start in
// lower case, to go inside other sentences.
static NSString *sentenceCase(NSString *s) {
	return s.length == 0 ? s : [[s substringToIndex:1].uppercaseString stringByAppendingString:[s substringFromIndex:1]];
}


@implementation LSLPeers
- (instancetype)initWithRows:(id)rows {
	self = [super init];
	self.peers = [NSMutableArray array];
	self.shown = [NSMutableArray array];
	if ([rows isKindOfClass:[NSArray class]]) {
		for (id r in rows) {
			if (![r isKindOfClass:[NSDictionary class]]) {
				continue;
			}
			NSMutableArray *models = [NSMutableArray array];
			id ids = r[@"models"];
			if ([ids isKindOfClass:[NSArray class]]) {
				for (id m in ids) {
					if ([m isKindOfClass:[NSString class]]) {
						[models addObject:[@{@"id" : m, @"on" : @YES} mutableCopy]];
					}
				}
			}
			[self.peers addObject:[@{
				@"name" : string(r, @"name"),
				@"url" : string(r, @"url"),
				@"key" : string(r, @"keyVariable"),
				@"models" : models,
			} mutableCopy]];
		}
	}
	return self;
}

- (NSMutableDictionary *)selected {
	NSInteger row = self.table.selectedRow;
	return row >= 0 && row < (NSInteger)self.peers.count ? self.peers[(NSUInteger)row] : nil;
}

- (NSInteger)numberOfRowsInTableView:(NSTableView *)tableView {
	return (NSInteger)(tableView == self.table ? self.peers.count : self.shown.count);
}

- (NSView *)tableView:(NSTableView *)tableView viewForTableColumn:(NSTableColumn *)column row:(NSInteger)row {
	if (tableView == self.table) {
		BOOL name = [column.identifier isEqual:@"name"];
		NSTextField *f = [NSTextField textFieldWithString:self.peers[(NSUInteger)row][name ? @"name" : @"url"]];
		f.placeholderString = name ? @"studio" : @"http://192.168.1.20:8080";
		f.bordered = NO;
		f.drawsBackground = NO;
		// Each cell is a field of its own, which disabling the table
		// doesn't reach: it's editable only while the tab is usable.
		f.editable = self.managed;
		f.selectable = self.managed;
		f.target = self;
		f.action = name ? @selector(nameEdited:) : @selector(urlEdited:);
		f.cell.sendsActionOnEndEditing = YES;
		f.cell.scrollable = YES;
		return f;
	}
	NSMutableDictionary *m = self.shown[(NSUInteger)row];
	if ([column.identifier isEqual:@"on"]) {
		NSButton *b = [NSButton checkboxWithTitle:@"" target:self action:@selector(toggled:)];
		b.state = [m[@"on"] boolValue] ? NSControlStateValueOn : NSControlStateValueOff;
		b.enabled = self.managed;
		return b;
	}
	NSTextField *l = [NSTextField labelWithString:m[@"id"]];
	l.lineBreakMode = NSLineBreakByTruncatingMiddle;
	l.toolTip = m[@"id"]; // the whole ID, when it's cut short
	return l;
}

- (void)nameEdited:(NSTextField *)sender {
	NSInteger row = [self.table rowForView:sender];
	if (row >= 0 && row < (NSInteger)self.peers.count) {
		self.peers[(NSUInteger)row][@"name"] = [sender.stringValue stringByTrimmingCharactersInSet:NSCharacterSet.whitespaceCharacterSet];
	}
}

- (void)urlEdited:(NSTextField *)sender {
	NSInteger row = [self.table rowForView:sender];
	if (row >= 0 && row < (NSInteger)self.peers.count) {
		self.peers[(NSUInteger)row][@"url"] = [sender.stringValue stringByTrimmingCharactersInSet:NSCharacterSet.whitespaceCharacterSet];
	}
}

- (void)tableViewSelectionDidChange:(NSNotification *)notification {
	if (notification.object == self.table) {
		[self showSelected];
	}
}

// say puts a message under the Models row, with all of it in the tooltip
// in case two lines cut it short.
- (void)say:(NSString *)message {
	self.status.stringValue = message;
	self.status.toolTip = message.length > 0 ? message : nil;
}

// showSelected shows the selected peer's key and models.
- (void)showSelected {
	self.filter.stringValue = @"";
	[self say:@""];
	[self refreshKeys];
	[self applyFilter];
	[self enable];
}

- (void)enable {
	for (NSControl *c in self.all) {
		c.enabled = self.managed;
	}
	BOOL detail = self.managed && self.selected != nil;
	for (NSControl *c in self.detail) {
		c.enabled = detail;
	}
	// The cells follow too: the checkboxes, and the peers' names and URLs.
	// Those are redrawn in place: reloading the table, or selecting a row
	// again, would report a selection change, which comes back here.
	[self.list reloadData];
	if (self.table.numberOfRows > 0) {
		[self.table reloadDataForRowIndexes:[NSIndexSet indexSetWithIndexesInRange:NSMakeRange(0, (NSUInteger)self.table.numberOfRows)]
		                      columnIndexes:[NSIndexSet indexSetWithIndexesInRange:NSMakeRange(0, 2)]];
	}
}

- (void)setManaged:(BOOL)managed {
	_managed = managed;
	self.managedHint.stringValue = managed
		? @"The app writes these to llama-swap's peers, in its managed config folder. Keep each peer in one place: "
		  @"a peer also in your own config stops llama-swap from loading."
		: @"Needs Config folder: Managed by the app, on the General tab. Until then these aren't used; "
		  @"peers in llama-swap's own config work as before.";
	[self enable];
}

// refreshKeys lists None and the secret variables in the key menu, and
// selects the selected peer's. A key that's no longer a secret variable
// stays, marked, so Save can say what's wrong.
- (void)refreshKeys {
	NSString *current = self.selected[@"key"] ?: @"";
	[self.key removeAllItems];
	[self.key addItemWithTitle:@"None"];
	self.key.lastItem.representedObject = @"";
	for (NSDictionary *v in self.vars.rows) {
		NSString *name = v[@"name"];
		if (name.length > 0 && [self.key indexOfItemWithRepresentedObject:name] < 0) {
			[self.key addItemWithTitle:name];
			self.key.lastItem.representedObject = name;
		}
	}
	if (current.length > 0 && [self.key indexOfItemWithRepresentedObject:current] < 0) {
		[self.key addItemWithTitle:[current stringByAppendingString:@" (not a secret variable)"]];
		self.key.lastItem.representedObject = current;
	}
	[self.key selectItemAtIndex:[self.key indexOfItemWithRepresentedObject:current]];
}

- (void)keyChanged:(id)sender {
	self.selected[@"key"] = self.key.selectedItem.representedObject ?: @"";
}

- (void)filterChanged:(id)sender {
	[self applyFilter];
}

// applyFilter shows the selected peer's models whose IDs contain the
// filter's text.
- (void)applyFilter {
	NSString *text = self.filter.stringValue;
	[self.shown removeAllObjects];
	for (NSMutableDictionary *m in self.selected[@"models"]) {
		if (text.length == 0 || [m[@"id"] localizedCaseInsensitiveContainsString:text]) {
			[self.shown addObject:m];
		}
	}
	[self.list reloadData];
	[self updateCount];
}

- (void)updateCount {
	NSArray *models = self.selected[@"models"];
	NSUInteger on = 0;
	for (NSDictionary *m in models) {
		on += [m[@"on"] boolValue] ? 1 : 0;
	}
	self.count.stringValue = models.count == 0 ? @""
		: self.filter.stringValue.length > 0
		? [NSString stringWithFormat:@"%lu of %lu ticked, %lu shown", (unsigned long)on, (unsigned long)models.count, (unsigned long)self.shown.count]
		: [NSString stringWithFormat:@"%lu of %lu ticked", (unsigned long)on, (unsigned long)models.count];
}

- (void)toggled:(NSButton *)sender {
	NSInteger row = [self.list rowForView:sender];
	if (row >= 0 && row < (NSInteger)self.shown.count) {
		self.shown[(NSUInteger)row][@"on"] = sender.state == NSControlStateValueOn ? @YES : @NO;
		[self updateCount];
	}
}

- (void)tickShown:(id)sender {
	[self markShown:YES];
}
- (void)untickShown:(id)sender {
	[self markShown:NO];
}
- (void)markShown:(BOOL)on {
	for (NSMutableDictionary *m in self.shown) {
		m[@"on"] = on ? @YES : @NO;
	}
	[self.list reloadData];
	[self updateCount];
}

static void sortModels(NSMutableArray<NSMutableDictionary *> *models) {
	[models sortUsingComparator:^NSComparisonResult(NSDictionary *a, NSDictionary *b) {
		return [a[@"id"] caseInsensitiveCompare:b[@"id"]];
	}];
}

// addModel adds, ticked, a model the peer doesn't list, typed by hand.
- (void)addModel:(id)sender {
	NSMutableDictionary *peer = self.selected;
	NSString *id_ = [self.addField.stringValue stringByTrimmingCharactersInSet:NSCharacterSet.whitespaceCharacterSet];
	if (peer == nil || id_.length == 0) {
		return;
	}
	NSMutableArray *models = peer[@"models"];
	NSMutableDictionary *found = nil;
	for (NSMutableDictionary *m in models) {
		if ([m[@"id"] isEqual:id_]) {
			found = m;
		}
	}
	if (found != nil) {
		found[@"on"] = @YES;
	} else {
		[models addObject:[@{@"id" : id_, @"on" : @YES} mutableCopy]];
		sortModels(models);
	}
	self.addField.stringValue = @"";
	[self applyFilter];
}

// Return in the filter or the add field would otherwise press Save.
- (BOOL)control:(NSControl *)control textView:(NSTextView *)textView doCommandBySelector:(SEL)command {
	if (command != @selector(insertNewline:)) {
		return NO;
	}
	if (control == self.addField) {
		[self addModel:nil];
	}
	return control == self.addField || control == self.filter;
}

// fetch asks the peer for its models, with the selected key's value as it
// stands in the Secrets table, so a key added a moment ago works before
// Save. New models arrive unticked; ticks already made stay.
- (void)fetch:(id)sender {
	NSMutableDictionary *peer = self.selected;
	if (peer == nil) {
		return;
	}
	[self.table.window makeFirstResponder:nil]; // finish an edit of the URL first
	NSString *url = peer[@"url"], *keyName = peer[@"key"], *key = @"";
	for (NSDictionary *v in self.vars.rows) {
		if (keyName.length > 0 && [v[@"name"] isEqual:keyName]) {
			key = v[@"value"];
		}
	}
	if (url.length == 0) {
		[self say:@"Give the peer's URL first."];
		return;
	}
	self.fetch.enabled = NO;
	[self.spinner startAnimation:nil];
	[self say:@"Asking the peer…"];
	__weak LSLPeers *weak = self;
	__weak NSMutableDictionary *weakPeer = peer;
	dispatch_async(dispatch_get_global_queue(QOS_CLASS_USER_INITIATED, 0), ^{
		char *reply = lsl_fetch_peer_models((char *)url.UTF8String, (char *)key.UTF8String);
		NSString *json = reply != NULL ? [NSString stringWithUTF8String:reply] : @"{}";
		free(reply);
		main_async(^{
			[weak fetched:json forPeer:weakPeer];
		});
	});
}

- (void)fetched:(NSString *)json forPeer:(NSMutableDictionary *)peer {
	[self.spinner stopAnimation:nil];
	[self enable];
	if (peer == nil) {
		return; // removed meanwhile
	}
	NSDictionary *reply = [NSJSONSerialization JSONObjectWithData:[json dataUsingEncoding:NSUTF8StringEncoding] options:0 error:nil];
	NSString *error = [reply isKindOfClass:[NSDictionary class]] ? string(reply, @"error") : @"no reply";
	NSArray *ids = [reply isKindOfClass:[NSDictionary class]] ? reply[@"models"] : nil;
	if (error.length > 0 || ![ids isKindOfClass:[NSArray class]]) {
		[self say:error.length > 0 ? sentenceCase(error) : @"The peer's reply couldn't be read."];
		return;
	}
	NSMutableArray *models = peer[@"models"];
	NSMutableSet *known = [NSMutableSet set];
	for (NSDictionary *m in models) {
		[known addObject:m[@"id"]];
	}
	NSUInteger added = 0;
	for (id i in ids) {
		if ([i isKindOfClass:[NSString class]] && ![known containsObject:i]) {
			[models addObject:[@{@"id" : i, @"on" : @NO} mutableCopy]];
			[known addObject:i];
			added++;
		}
	}
	sortModels(models);
	if (peer == self.selected) {
		[self applyFilter];
	}
	[self say:[NSString stringWithFormat:@"It serves %lu models; %lu new, unticked.", (unsigned long)ids.count, (unsigned long)added]];
}

- (void)addPeer:(id)sender {
	[self.table.window makeFirstResponder:nil];
	[self.peers addObject:[@{@"name" : @"", @"url" : @"", @"key" : @"", @"models" : [NSMutableArray array]} mutableCopy]];
	NSInteger row = (NSInteger)self.peers.count - 1;
	[self.table reloadData];
	[self.table selectRowIndexes:[NSIndexSet indexSetWithIndex:(NSUInteger)row] byExtendingSelection:NO];
	[self.table editColumn:0 row:row withEvent:nil select:YES];
}

- (void)removePeer:(id)sender {
	NSInteger row = self.table.selectedRow;
	if (row < 0) {
		return;
	}
	[self.table.window makeFirstResponder:nil];
	[self.peers removeObjectAtIndex:(NSUInteger)row];
	[self.table reloadData];
	[self showSelected];
}

// json is the peers, each with only its ticked models, for the reply.
- (NSArray *)json {
	NSMutableArray *out = [NSMutableArray array];
	for (NSDictionary *p in self.peers) {
		NSMutableArray *models = [NSMutableArray array];
		for (NSDictionary *m in p[@"models"]) {
			if ([m[@"on"] boolValue]) {
				[models addObject:m[@"id"]];
			}
		}
		[out addObject:@{@"name" : p[@"name"], @"url" : p[@"url"], @"keyVariable" : p[@"key"], @"models" : models}];
	}
	return out;
}
@end

// peersTab lays out the Peers tab for p.
static NSView *peersTab(LSLPeers *p) {
	NSTableView *table = [[NSTableView alloc] init];
	NSTableColumn *name = [[NSTableColumn alloc] initWithIdentifier:@"name"];
	name.title = @"Name";
	name.width = 150;
	NSTableColumn *url = [[NSTableColumn alloc] initWithIdentifier:@"url"];
	url.title = @"URL";
	url.width = 370;
	[table addTableColumn:name];
	[table addTableColumn:url];
	table.dataSource = p;
	table.delegate = p;
	table.usesAlternatingRowBackgroundColors = YES;
	table.columnAutoresizingStyle = NSTableViewLastColumnOnlyAutoresizingStyle;
	p.table = table;
	NSScrollView *peerScroll = [[NSScrollView alloc] init];
	peerScroll.documentView = table;
	peerScroll.hasVerticalScroller = YES;
	peerScroll.borderType = NSBezelBorder;
	[peerScroll.widthAnchor constraintEqualToConstant:540].active = YES;
	[peerScroll.heightAnchor constraintEqualToConstant:80].active = YES;
	NSButton *add = smallButton(NSImageNameAddTemplate, @"Add a peer", p, @selector(addPeer:));
	NSButton *remove = smallButton(NSImageNameRemoveTemplate, @"Remove the selected peer", p, @selector(removePeer:));

	p.key = [[NSPopUpButton alloc] init];
	p.key.target = p;
	p.key.action = @selector(keyChanged:);

	p.filter = [[NSSearchField alloc] init];
	p.filter.placeholderString = @"Filter";
	p.filter.sendsSearchStringImmediately = YES;
	p.filter.target = p;
	p.filter.action = @selector(filterChanged:);
	p.filter.delegate = p;
	[p.filter.widthAnchor constraintEqualToConstant:200].active = YES;
	p.fetch = [NSButton buttonWithTitle:@"Fetch Models" target:p action:@selector(fetch:)];
	p.spinner = [[NSProgressIndicator alloc] init];
	p.spinner.style = NSProgressIndicatorStyleSpinning;
	p.spinner.controlSize = NSControlSizeSmall;
	p.spinner.displayedWhenStopped = NO;
	// On its own line, the tab's width, up to two lines. Two lines of text
	// to start with, so the tab is sized for them; showSelected clears it.
	p.status = [NSTextField wrappingLabelWithString:@" \n "];
	p.status.font = [NSFont systemFontOfSize:NSFont.smallSystemFontSize];
	p.status.textColor = NSColor.secondaryLabelColor;
	p.status.maximumNumberOfLines = 2;
	p.status.lineBreakMode = NSLineBreakByTruncatingTail;
	p.status.preferredMaxLayoutWidth = 540;

	NSTableView *list = [[NSTableView alloc] init];
	NSTableColumn *on = [[NSTableColumn alloc] initWithIdentifier:@"on"];
	on.title = @"";
	on.width = 22;
	NSTableColumn *model = [[NSTableColumn alloc] initWithIdentifier:@"id"];
	model.title = @"Model";
	model.width = 490;
	[list addTableColumn:on];
	[list addTableColumn:model];
	list.dataSource = p;
	list.delegate = p;
	list.usesAlternatingRowBackgroundColors = YES;
	list.columnAutoresizingStyle = NSTableViewLastColumnOnlyAutoresizingStyle;
	p.list = list;
	NSScrollView *listScroll = [[NSScrollView alloc] init];
	listScroll.documentView = list;
	listScroll.hasVerticalScroller = YES;
	listScroll.borderType = NSBezelBorder;
	[listScroll.widthAnchor constraintEqualToConstant:540].active = YES;
	[listScroll.heightAnchor constraintEqualToConstant:170].active = YES;

	NSButton *tick = [NSButton buttonWithTitle:@"Tick All Shown" target:p action:@selector(tickShown:)];
	NSButton *untick = [NSButton buttonWithTitle:@"Untick All Shown" target:p action:@selector(untickShown:)];
	p.count = label(@"");
	p.count.textColor = NSColor.secondaryLabelColor;
	p.addField = field(@"", @"A model it doesn't list", 260);
	p.addField.delegate = p;
	NSButton *addModel = [NSButton buttonWithTitle:@"Add" target:p action:@selector(addModel:)];

	// The longer of its two texts, for sizing the tab; setManaged sets the
	// real one.
	p.managedHint = hint(@"The app writes these to llama-swap's peers, in its managed config folder. Keep each peer in one place: "
	                     @"a peer also in your own config stops llama-swap from loading.");
	p.detail = @[ remove, p.key, p.filter, p.fetch, list, tick, untick, p.addField, addModel ];
	p.all = @[ table, add ];

	NSTextField *keyHint = label(@"one of the secret variables on the Secrets tab, or None");
	keyHint.font = [NSFont systemFontOfSize:NSFont.smallSystemFontSize];
	keyHint.textColor = NSColor.secondaryLabelColor;
	NSTextField *title = label(@"Peers");
	title.font = [NSFont boldSystemFontOfSize:NSFont.systemFontSize];
	NSStackView *v = column(@[
		title,
		hint(@"Other llama-swaps, or servers with the same API, whose models llama-swap offers as name/model."),
		peerScroll,
		row(@[ add, remove ]),
		row(@[ label(@"Key:"), p.key, keyHint ]),
		row(@[ label(@"Models:"), p.filter, p.fetch, p.spinner ]),
		p.status,
		listScroll,
		row(@[ tick, untick, p.count ]),
		row(@[ p.addField, addModel ]),
		p.managedHint,
	]);
	[v setCustomSpacing:12 afterView:v.views[3]];
	[v setCustomSpacing:12 afterView:v.views[4]];
	return v;
}

char *lsl_preferences(const char *json, int *result) {
	NSData *data = [str(json) dataUsingEncoding:NSUTF8StringEncoding];
	NSDictionary *in = [NSJSONSerialization JSONObjectWithData:data options:0 error:nil];
	if (![in isKindOfClass:[NSDictionary class]]) {
		in = @{};
	}
	__block char *out = NULL;
	__block int res = 0;
	on_main(^{
		ensure_edit_menu();
		LSLPrefsController *c = [[LSLPrefsController alloc] init];

		// General.
		c.binary = field(string(in, @"binary"), string(in, @"foundBinary"), 340);
		c.config = field(string(in, @"config"), @"", 340);
		NSButton *chooseBinary = [NSButton buttonWithTitle:@"Choose…" target:c action:@selector(chooseBinary:)];
		NSButton *chooseConfig = [NSButton buttonWithTitle:@"Choose…" target:c action:@selector(chooseConfig:)];
		NSTextField *listen = field(string(in, @"listen"), @"127.0.0.1:8080", 180);
		NSTextView *env;
		NSScrollView *envArea = area(string(in, @"env"), c, &env);
		c.dirMode = [[NSPopUpButton alloc] init];
		NSArray *dirModes = @[ @[ @"off", @"None" ], @[ @"own", @"Your own folder" ], @[ @"managed", @"Managed by the app" ] ];
		for (NSArray *m in dirModes) {
			[c.dirMode addItemWithTitle:m[1]];
			c.dirMode.lastItem.representedObject = m[0];
		}
		NSInteger dirMode = [c.dirMode indexOfItemWithRepresentedObject:string(in, @"configDirMode")];
		[c.dirMode selectItemAtIndex:dirMode >= 0 ? dirMode : 0];
		c.dirMode.target = c;
		c.dirMode.action = @selector(dirModeChanged:);
		c.configDir = field(@"", @"", 340);
		c.ownDir = string(in, @"configDir");
		c.managedDir = string(in, @"managedConfigDir");
		c.chooseDir = [NSButton buttonWithTitle:@"Choose…" target:c action:@selector(chooseConfigDir:)];
		c.showDir = [NSButton buttonWithTitle:@"Show in Finder" target:c action:@selector(showConfigDir:)];
		NSButton *watch = checkbox(@"Reload the config when it changes", in[@"watchConfig"]);
		NSTextField *seconds = field(string(in, @"healthCheckSeconds"), @"60", 60);
		NSButton *skip = checkbox(@"Skip a check when llama-swap has been busy since the last one", in[@"healthCheckSkipWhenActive"]);
		NSButton *autoStart = checkbox(@"Start llama-swap when the app opens", in[@"autoStart"]);
		NSButton *unloadFaulted = checkbox(@"Unload a model as soon as its GPU backend fails, instead of asking", in[@"unloadOnGpuFault"]);
		NSButton *markUpdated = checkbox(@"Mark models whose server program was updated since they loaded", in[@"markUpdatedModels"]);
		NSView *empty = NSGridCell.emptyContentView;
		NSGridView *general = grid(@[
			@[ label(@"llama-swap binary:"), row(@[ c.binary, chooseBinary ]) ],
			@[ empty, hint(@"Leave empty to look in the usual install locations.") ],
			@[ label(@"Config file:"), row(@[ c.config, chooseConfig ]) ],
			@[ label(@"Config folder:"), c.dirMode ],
			@[ empty, row(@[ c.configDir, c.chooseDir, c.showDir ]) ],
			@[ empty, hint(@"llama-swap also reads the folder's .yaml files. Managed by the app is a folder the app keeps "
			                @"for the settings it writes for llama-swap, such as Require these keys (Secrets tab).") ],
			@[ empty, watch ],
			@[ label(@"Listen address:"), listen ],
			@[ empty, hint(@"Use 0.0.0.0:8080 for other machines to reach llama-swap. They can then use all of it, "
			                @"including unloading models and reading logs, unless its config sets API keys (Secrets tab).") ],
			@[ label(@"Environment:"), envArea ],
			@[ empty, hint(@"NAME=value, one per line, e.g. HF_TOKEN=…. Used exactly as typed, quotes included: "
			                @"for a path with spaces, quote it where llama-swap's config uses it.") ],
			@[ label(@"Health check every:"), row(@[ seconds, label(@"seconds") ]) ],
			@[ empty, skip ],
			@[ empty, unloadFaulted ],
			@[ empty, markUpdated ],
			@[ empty, autoStart ],
		]);
		topAligned(general, 9);

		// Logs.
		c.launcherLog = field(string(in, @"launcherLog"), string(in, @"defaultLauncherLog"), 340);
		c.ownLog = field(string(in, @"llamaSwapLogFile"), string(in, @"defaultLlamaSwapLog"), 340);
		NSButton *chooseLauncherLog = [NSButton buttonWithTitle:@"Choose…" target:c action:@selector(chooseLauncherLog:)];
		c.chooseOwnLog = [NSButton buttonWithTitle:@"Choose…" target:c action:@selector(chooseOwnLog:)];
		c.logMode = [[NSPopUpButton alloc] init];
		NSArray *modes = @[ @[ @"launcher", @"In the launcher log" ], @[ @"file", @"In its own file" ], @[ @"off", @"Not saved" ] ];
		for (NSArray *m in modes) {
			[c.logMode addItemWithTitle:m[1]];
			c.logMode.lastItem.representedObject = m[0];
		}
		NSInteger mode = [c.logMode indexOfItemWithRepresentedObject:string(in, @"llamaSwapLog")];
		[c.logMode selectItemAtIndex:mode >= 0 ? mode : 0];
		c.logMode.target = c;
		c.logMode.action = @selector(logModeChanged:);
		[c logModeChanged:nil];
		NSGridView *logs = grid(@[
			@[ label(@"Launcher log:"), row(@[ c.launcherLog, chooseLauncherLog ]) ],
			@[ empty, hint(@"The app's own notes and crash reports, and llama-swap's output when it goes here.") ],
			@[ label(@"llama-swap's output:"), c.logMode ],
			@[ label(@"llama-swap log:"), row(@[ c.ownLog, c.chooseOwnLog ]) ],
			@[ empty, hint(@"Exactly as llama-swap writes it, nothing added, for a log collector to read, say. "
			               @"When it isn't saved, the app still keeps its last lines in memory, for alerts and llsl.") ],
			@[ empty, hint(@"Each log is rotated at 10 MB, keeping 5 old files. A log in Desktop, Documents, Downloads, "
			               @"iCloud Drive or on an external disk makes macOS ask the app for access, once.") ],
		]);

		// Secrets: the API keys, then the secret environment variables.
		c.keys = [[LSLSecretTable alloc] initWithRows:in[@"apiKeys"]];
		c.keys.prefix = string(in, @"keyPrefix");
		c.keys.capitals = YES;
		c.keys.generates = YES;
		c.keys.placeholder = @"NAME";
		__weak LSLPrefsController *weak = c;
		c.keys.namesChanged = ^{
			[weak refreshAppKey];
		};
		c.vars = [[LSLSecretTable alloc] initWithRows:in[@"secretEnv"]];
		c.vars.placeholder = @"HF_TOKEN";
		c.vars.namesChanged = ^{
			[weak.peers refreshKeys]; // a peer's key is one of these
		};
		c.appKey = [[NSPopUpButton alloc] init];
		[c refreshAppKey];
		NSInteger chosen = [c.appKey indexOfItemWithRepresentedObject:string(in, @"appApiKey")];
		if (chosen >= 0) {
			[c.appKey selectItemAtIndex:chosen];
		}
		c.keysExample = [NSString stringWithFormat:@"apiKeys: [\"${env.%@NAME}\", …]", c.keys.prefix];
		c.manageKeys = checkbox(@"Require these keys in llama-swap", in[@"manageApiKeys"]);
		// The longer of its two texts, for sizing the tab; dirModeChanged
		// sets the real one.
		c.manageKeysHint = hint([NSString stringWithFormat:@"Needs Config folder: Managed by the app, on the General tab. "
		                                                   @"Until then, list them in llama-swap's config yourself:\n%@", c.keysExample]);
		NSTextField *keysTitle = label(@"API keys");
		keysTitle.font = [NSFont boldSystemFontOfSize:NSFont.systemFontSize];
		NSTextField *varsTitle = label(@"Secret environment variables");
		varsTitle.font = keysTitle.font;
		NSStackView *secrets = column(@[
			keysTitle,
			secretTable(c.keys, @"Variable", @"Key", 110),
			c.manageKeys,
			c.manageKeysHint,
			row(@[ label(@"The app uses:"), c.appKey ]),
			hint(@"Any of the keys works; the app sends this one on its own requests."),
			varsTitle,
			secretTable(c.vars, @"Variable", @"Value", 90),
			hint(@"Like Environment on the General tab, for values to keep out of files, such as HF_TOKEN."),
			hint(@"All of these are kept in your login keychain."),
		]);
		[secrets setCustomSpacing:16 afterView:secrets.views[5]];

		// Peers.
		c.peers = [[LSLPeers alloc] initWithRows:in[@"peers"]];
		c.peers.vars = c.vars;
		NSView *peersView = peersTab(c.peers);

		// The tabs, sized to fit the larger page: an alert sizes itself to
		// its accessory view's frame.
		NSTabView *tabs = [[NSTabView alloc] init];
		[tabs addTabViewItem:tab(@"general", @"General", general)];
		[tabs addTabViewItem:tab(@"secrets", @"Secrets", secrets)];
		[tabs addTabViewItem:tab(@"peers", @"Peers", peersView)];
		[tabs addTabViewItem:tab(@"logs", @"Logs", logs)];
		NSSize content = NSZeroSize;
		for (NSTabViewItem *item in tabs.tabViewItems) {
			NSSize s = item.view.fittingSize;
			content = NSMakeSize(MAX(content.width, s.width), MAX(content.height, s.height));
		}
		tabs.frame = NSMakeRect(0, 0, 100, 100);
		NSSize chrome = NSMakeSize(100 - tabs.contentRect.size.width, 100 - tabs.contentRect.size.height);
		tabs.frame = NSMakeRect(0, 0, content.width + chrome.width, content.height + chrome.height);
		if (c.peers.peers.count > 0) {
			[c.peers.table selectRowIndexes:[NSIndexSet indexSetWithIndex:0] byExtendingSelection:NO];
		}
		[c.peers showSelected];
		[c dirModeChanged:nil];
		NSString *first = string(in, @"tab");
		if ([first isEqual:@"secrets"] || [first isEqual:@"peers"] || [first isEqual:@"logs"]) {
			[tabs selectTabViewItemWithIdentifier:first];
		}

		NSAlert *alert = [[NSAlert alloc] init];
		alert.messageText = @"Settings";
		NSString *problem = string(in, @"error");
		alert.informativeText = problem.length > 0
			? [NSString stringWithFormat:@"⚠︎ %@", problem]
			: [NSString stringWithFormat:@"Saved in %@", string(in, @"file").stringByAbbreviatingWithTildeInPath];
		alert.accessoryView = tabs;
		[alert addButtonWithTitle:@"Save"];   // the default: Return
		[alert addButtonWithTitle:@"Cancel"]; // Escape
		[alert layout];
		NSString *shown = tabs.selectedTabViewItem.identifier;
		alert.window.initialFirstResponder = [shown isEqual:@"secrets"] ? (NSView *)c.keys.table
		                                     : [shown isEqual:@"peers"] ? (NSView *)c.peers.table
		                                     : [shown isEqual:@"logs"]  ? (NSView *)c.launcherLog
		                                                                 : (NSView *)c.binary;
		[NSApp activate];
		NSModalResponse r = lsl_run_modal(alert);
		res = r == NSAlertFirstButtonReturn ? 1 : 0; // Cancel, or closed by lsl_close_dialogs
		// Finish an edit still in progress in a table cell, so it counts.
		[alert.window makeFirstResponder:nil];

		NSDictionary *fields = @{
			@"binary" : c.binary.stringValue,
			@"config" : c.config.stringValue,
			@"listen" : listen.stringValue,
			@"env" : env.string,
			@"watchConfig" : watch.state == NSControlStateValueOn ? @YES : @NO,
			@"configDirMode" : c.dirMode.selectedItem.representedObject ?: @"off",
			@"configDir" : [c ownDirValue] ?: @"",
			@"manageApiKeys" : c.manageKeys.state == NSControlStateValueOn ? @YES : @NO,
			@"healthCheckSeconds" : seconds.stringValue,
			// @YES and @NO, not @(a == b): a boxed comparison is a number,
			// which becomes JSON 1 or 0, and Go won't decode that into a bool.
			@"healthCheckSkipWhenActive" : skip.state == NSControlStateValueOn ? @YES : @NO,
			@"autoStart" : autoStart.state == NSControlStateValueOn ? @YES : @NO,
			@"unloadOnGpuFault" : unloadFaulted.state == NSControlStateValueOn ? @YES : @NO,
			@"markUpdatedModels" : markUpdated.state == NSControlStateValueOn ? @YES : @NO,
			@"apiKeys" : [c.keys json],
			@"appApiKey" : c.appKey.selectedItem.representedObject ?: @"",
			@"secretEnv" : [c.vars json],
			@"peers" : [c.peers json],
			@"launcherLog" : c.launcherLog.stringValue,
			@"llamaSwapLog" : c.logMode.selectedItem.representedObject ?: @"launcher",
			@"llamaSwapLogFile" : c.ownLog.stringValue,
			@"tab" : tabs.selectedTabViewItem.identifier ?: @"general",
		};
		NSData *encoded = [NSJSONSerialization dataWithJSONObject:fields options:0 error:nil];
		out = strdup([[NSString alloc] initWithData:encoded encoding:NSUTF8StringEncoding].UTF8String);
	});
	*result = res;
	return out;
}
