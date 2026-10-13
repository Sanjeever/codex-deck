#import <Cocoa/Cocoa.h>

extern void codexDeckTrayShow(void);
extern void codexDeckTrayQuit(void);

@interface CodexDeckTrayTarget : NSObject
- (void)showWindow:(id)sender;
- (void)quitApp:(id)sender;
@end

@implementation CodexDeckTrayTarget
- (void)showWindow:(id)sender { codexDeckTrayShow(); }
- (void)quitApp:(id)sender { codexDeckTrayQuit(); }
@end

static NSStatusItem *statusItem;
static CodexDeckTrayTarget *target;
static NSMenuItem *applicationQuitItem;
static id applicationQuitTarget;

static void onMainThread(void (^action)(void)) {
    if ([NSThread isMainThread]) {
        action();
    } else {
        dispatch_sync(dispatch_get_main_queue(), action);
    }
}

bool codexDeckTrayStart(const unsigned char *icon, int length) {
    __block bool ready = false;
    onMainThread(^{
        NSData *data = [NSData dataWithBytes:icon length:length];
        NSImage *image = [[NSImage alloc] initWithData:data];
        if (image == nil) return;
        [image setSize:NSMakeSize(18, 18)];
        statusItem = [[NSStatusBar systemStatusBar] statusItemWithLength:NSSquareStatusItemLength];
        if (statusItem == nil) {
            [image release];
            return;
        }
        [statusItem retain];
        statusItem.button.image = image;
        statusItem.button.toolTip = @"Codex Deck";
        [image release];
        target = [[CodexDeckTrayTarget alloc] init];
        // 将应用菜单和 Cmd+Q 接入显式退出，关闭窗口仍只隐藏。
        NSMenu *applicationMenu = [[NSApp.mainMenu itemAtIndex:0] submenu];
        for (NSMenuItem *item in applicationMenu.itemArray) {
            if (item.action == NSSelectorFromString(@"Quit")) {
                applicationQuitItem = [item retain];
                applicationQuitTarget = [item.target retain];
                item.target = target;
                item.action = @selector(quitApp:);
                break;
            }
        }
        NSMenu *menu = [[NSMenu alloc] initWithTitle:@"Codex Deck"];
        NSMenuItem *show = [menu addItemWithTitle:@"显示窗口" action:@selector(showWindow:) keyEquivalent:@""];
        show.target = target;
        NSMenuItem *quit = [menu addItemWithTitle:@"退出" action:@selector(quitApp:) keyEquivalent:@""];
        quit.target = target;
        statusItem.menu = menu;
        [menu release];
        ready = true;
    });
    return ready;
}

void codexDeckTrayStop(void) {
    onMainThread(^{
        applicationQuitItem.target = applicationQuitTarget;
        applicationQuitItem.action = NSSelectorFromString(@"Quit");
        [applicationQuitItem release];
        applicationQuitItem = nil;
        [applicationQuitTarget release];
        applicationQuitTarget = nil;
        [[NSStatusBar systemStatusBar] removeStatusItem:statusItem];
        [statusItem release];
        statusItem = nil;
        [target release];
        target = nil;
    });
}
