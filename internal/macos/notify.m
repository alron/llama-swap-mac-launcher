// macOS notifications, for the Go code in notify.go (TODO item 60).
//
// UNUserNotificationCenter is safe to use from any thread, so unlike the
// other shims these don't switch to the main thread. Its calls report back
// through completion blocks on its own queue; the C functions wait for them
// with a semaphore, since Go calls them from goroutines that can block.

#import <UserNotifications/UserNotifications.h>

#include "common.h"
#include "notify.h"

// Clicks on notifications, oldest first, waiting for Go to take them.
static NSMutableArray<NSArray<NSString *> *> *responses;
static dispatch_semaphore_t responseReady;

@interface LSLNotifyDelegate : NSObject <UNUserNotificationCenterDelegate>
@end

@implementation LSLNotifyDelegate
// While the app is active (it activates itself for its dialogs), macOS
// shows a notification only if the delegate says how; otherwise it's
// silently dropped.
- (void)userNotificationCenter:(UNUserNotificationCenter *)center
       willPresentNotification:(UNNotification *)notification
         withCompletionHandler:(void (^)(UNNotificationPresentationOptions))done {
	done(UNNotificationPresentationOptionBanner | UNNotificationPresentationOptionList | UNNotificationPresentationOptionSound);
}

// A click on a notification, or on one of its buttons.
- (void)userNotificationCenter:(UNUserNotificationCenter *)center
didReceiveNotificationResponse:(UNNotificationResponse *)response
         withCompletionHandler:(void (^)(void))done {
	NSString *action = [response.actionIdentifier isEqual:UNNotificationDefaultActionIdentifier] ? @"default" : response.actionIdentifier;
	@synchronized(responses) {
		[responses addObject:@[ response.notification.request.identifier, action ]];
	}
	dispatch_semaphore_signal(responseReady);
	done();
}
@end

// The centre keeps only a weak reference to its delegate.
static LSLNotifyDelegate *delegate;

void lsl_notify_setup(void) {
	responses = [NSMutableArray array];
	responseReady = dispatch_semaphore_create(0);
	delegate = [[LSLNotifyDelegate alloc] init];
	UNUserNotificationCenter *center = UNUserNotificationCenter.currentNotificationCenter;
	center.delegate = delegate;
	UNNotificationAction *unload = [UNNotificationAction actionWithIdentifier:@"unload" title:@"Unload" options:UNNotificationActionOptionNone];
	UNNotificationCategory *fault = [UNNotificationCategory categoryWithIdentifier:@"gpu-fault"
	                                                                        actions:@[ unload ]
	                                                              intentIdentifiers:@[]
	                                                                        options:UNNotificationCategoryOptionNone];
	[center setNotificationCategories:[NSSet setWithObject:fault]];
}

int lsl_notify_authorize(char **err) {
	__block BOOL allowed = NO;
	__block NSError *failure = nil;
	dispatch_semaphore_t done = dispatch_semaphore_create(0);
	[UNUserNotificationCenter.currentNotificationCenter
		requestAuthorizationWithOptions:UNAuthorizationOptionAlert | UNAuthorizationOptionSound
		              completionHandler:^(BOOL granted, NSError *error) {
			              allowed = granted;
			              failure = error;
			              dispatch_semaphore_signal(done);
		              }];
	dispatch_semaphore_wait(done, DISPATCH_TIME_FOREVER);
	*err = failure != nil ? strdup(failure.localizedDescription.UTF8String) : NULL;
	return allowed ? 1 : 0;
}

char *lsl_notify_post(const char *id, const char *title, const char *body, const char *category, int time_sensitive) {
	UNMutableNotificationContent *content = [[UNMutableNotificationContent alloc] init];
	content.title = str(title);
	content.body = str(body);
	content.categoryIdentifier = str(category);
	content.sound = UNNotificationSound.defaultSound;
	content.interruptionLevel = time_sensitive ? UNNotificationInterruptionLevelTimeSensitive : UNNotificationInterruptionLevelActive;
	UNNotificationRequest *request = [UNNotificationRequest requestWithIdentifier:str(id) content:content trigger:nil];
	__block NSError *failure = nil;
	dispatch_semaphore_t done = dispatch_semaphore_create(0);
	[UNUserNotificationCenter.currentNotificationCenter addNotificationRequest:request
	                                                     withCompletionHandler:^(NSError *error) {
		                                                     failure = error;
		                                                     dispatch_semaphore_signal(done);
	                                                     }];
	dispatch_semaphore_wait(done, DISPATCH_TIME_FOREVER);
	return failure != nil ? strdup(failure.localizedDescription.UTF8String) : NULL;
}

void lsl_notify_remove(const char *id) {
	NSArray *ids = @[ str(id) ];
	UNUserNotificationCenter *center = UNUserNotificationCenter.currentNotificationCenter;
	[center removeDeliveredNotificationsWithIdentifiers:ids];
	[center removePendingNotificationRequestsWithIdentifiers:ids];
}

void lsl_notify_remove_all(void) {
	UNUserNotificationCenter *center = UNUserNotificationCenter.currentNotificationCenter;
	[center removeAllDeliveredNotifications];
	[center removeAllPendingNotificationRequests];
}

char *lsl_notify_next_response(void) {
	dispatch_semaphore_wait(responseReady, DISPATCH_TIME_FOREVER);
	NSArray<NSString *> *r;
	@synchronized(responses) {
		r = responses.firstObject;
		[responses removeObjectAtIndex:0];
	}
	return strdup([NSString stringWithFormat:@"%@\n%@", r[0], r[1]].UTF8String);
}

// setting names a UNNotificationSetting for the JSON.
static NSString *setting(UNNotificationSetting s) {
	switch (s) {
	case UNNotificationSettingEnabled:
		return @"enabled";
	case UNNotificationSettingDisabled:
		return @"disabled";
	default:
		return @"not supported";
	}
}

char *lsl_notify_settings(void) {
	__block UNNotificationSettings *settings = nil;
	dispatch_semaphore_t done = dispatch_semaphore_create(0);
	[UNUserNotificationCenter.currentNotificationCenter getNotificationSettingsWithCompletionHandler:^(UNNotificationSettings *s) {
		settings = s;
		dispatch_semaphore_signal(done);
	}];
	dispatch_semaphore_wait(done, DISPATCH_TIME_FOREVER);
	NSArray *statuses = @[ @"not determined", @"denied", @"authorized", @"provisional", @"ephemeral" ];
	NSArray *styles = @[ @"none", @"banner", @"alert" ];
	NSInteger status = settings.authorizationStatus, style = settings.alertStyle;
	NSDictionary *d = @{
		@"authorization" : status >= 0 && status < (NSInteger)statuses.count ? statuses[(NSUInteger)status] : @"unknown",
		@"alerts" : setting(settings.alertSetting),
		@"style" : style >= 0 && style < (NSInteger)styles.count ? styles[(NSUInteger)style] : @"unknown",
		@"sound" : setting(settings.soundSetting),
		@"notificationCentre" : setting(settings.notificationCenterSetting),
		@"timeSensitive" : setting(settings.timeSensitiveSetting),
	};
	NSData *json = [NSJSONSerialization dataWithJSONObject:d options:NSJSONWritingSortedKeys error:nil];
	return strdup([[NSString alloc] initWithData:json encoding:NSUTF8StringEncoding].UTF8String);
}
