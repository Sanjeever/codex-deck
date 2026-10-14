#import <Cocoa/Cocoa.h>

extern void codexDeckTrayShow(void);
extern void codexDeckTrayQuit(void);
extern void codexDeckTrayRefresh(void);
extern void codexDeckTraySwitch(const char *id);

@interface CodexDeckTrayTarget : NSObject
- (void)showWindow:(id)sender;
- (void)quitApp:(id)sender;
- (void)refresh:(id)sender;
- (void)switchAccount:(id)sender;
@end

@implementation CodexDeckTrayTarget
- (void)showWindow:(id)sender { codexDeckTrayShow(); }
- (void)quitApp:(id)sender { codexDeckTrayQuit(); }
- (void)refresh:(id)sender { codexDeckTrayRefresh(); }
- (void)switchAccount:(NSMenuItem *)sender { codexDeckTraySwitch([sender.representedObject UTF8String]); }
@end

static NSStatusItem *statusItem;
static CodexDeckTrayTarget *target;
static NSMenuItem *applicationQuitItem;
static id applicationQuitTarget;
static NSMenuItem *currentItem, *quotaItem, *updatedItem, *statusMessageItem, *refreshItem, *switchItem;

static void onMainThread(void (^action)(void)) {
    if ([NSThread isMainThread]) {
        action();
    } else {
        dispatch_sync(dispatch_get_main_queue(), action);
    }
}

static void drawMenuBarCard(NSRect rect, CGFloat angle) {
    NSBezierPath *card = [NSBezierPath bezierPathWithRoundedRect:rect xRadius:1.8 yRadius:1.8];
    NSAffineTransform *transform = [NSAffineTransform transform];
    [transform translateXBy:NSMidX(rect) yBy:NSMidY(rect)];
    [transform rotateByDegrees:angle];
    [transform translateXBy:-NSMidX(rect) yBy:-NSMidY(rect)];
    [card transformUsingAffineTransform:transform];

    // 清除被前层卡片遮挡的线条，保留透明背景供系统着色。
    [NSGraphicsContext saveGraphicsState];
    NSGraphicsContext.currentContext.compositingOperation = NSCompositingOperationDestinationOut;
    [NSColor.blackColor setFill];
    [card fill];
    [NSGraphicsContext restoreGraphicsState];
    [NSColor.blackColor setStroke];
    card.lineWidth = 1.2;
    [card stroke];
}

static NSImage *menuBarIcon(void) {
    // 矢量绘制保持 Retina 清晰度，模板图像由系统按菜单栏外观着色。
    NSImage *image = [NSImage imageWithSize:NSMakeSize(18, 18) flipped:NO drawingHandler:^BOOL(NSRect rect) {
        drawMenuBarCard(NSMakeRect(1.9, 4.8, 11, 11), 12);
        drawMenuBarCard(NSMakeRect(3.5, 3.4, 11, 11), 5);
        drawMenuBarCard(NSMakeRect(5.1, 1.8, 11, 11), -7);

        [NSGraphicsContext saveGraphicsState];
        NSAffineTransform *transform = [NSAffineTransform transform];
        [transform translateXBy:10.6 yBy:7.3];
        [transform rotateByDegrees:-7];
        [transform translateXBy:-10.6 yBy:-7.3];
        [transform concat];
        NSBezierPath *prompt = [NSBezierPath bezierPath];
        prompt.lineWidth = 1.4;
        prompt.lineCapStyle = NSLineCapStyleRound;
        prompt.lineJoinStyle = NSLineJoinStyleRound;
        [prompt moveToPoint:NSMakePoint(8, 9.3)];
        [prompt lineToPoint:NSMakePoint(10.2, 7.5)];
        [prompt lineToPoint:NSMakePoint(8, 5.7)];
        [prompt moveToPoint:NSMakePoint(11.7, 5.7)];
        [prompt lineToPoint:NSMakePoint(13.5, 5.7)];
        [prompt stroke];
        [NSGraphicsContext restoreGraphicsState];
        return YES;
    }];
    image.template = YES;
    return image;
}

bool codexDeckTrayStart(void) {
    __block bool ready = false;
    onMainThread(^{
        NSImage *image = menuBarIcon();
        statusItem = [[NSStatusBar systemStatusBar] statusItemWithLength:NSSquareStatusItemLength];
        if (statusItem == nil) {
            return;
        }
        [statusItem retain];
        statusItem.button.image = image;
        statusItem.button.toolTip = @"Codex Deck";
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
        menu.autoenablesItems = NO;
        currentItem = [menu addItemWithTitle:@"未识别当前账号" action:nil keyEquivalent:@""];
        quotaItem = [menu addItemWithTitle:@"5H 未知 · Weekly 未知" action:nil keyEquivalent:@""];
        updatedItem = [menu addItemWithTitle:@"尚未查询" action:nil keyEquivalent:@""];
        statusMessageItem = [menu addItemWithTitle:@"" action:nil keyEquivalent:@""];
        for (NSMenuItem *item in @[currentItem, quotaItem, updatedItem, statusMessageItem]) item.enabled = NO;
        statusMessageItem.hidden = YES;
        refreshItem = [menu addItemWithTitle:@"刷新当前账号" action:@selector(refresh:) keyEquivalent:@""];
        refreshItem.target = target;
        refreshItem.enabled = NO;
        switchItem = [menu addItemWithTitle:@"切换账号" action:nil keyEquivalent:@""];
        switchItem.enabled = NO;
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

void codexDeckTrayUpdate(const char *payload) {
    NSString *json = [[NSString alloc] initWithUTF8String:payload];
    // 状态推送可能持有 Go 业务锁，异步更新避免主线程与业务线程互相等待。
    dispatch_async(dispatch_get_main_queue(), ^{
        if (statusItem != nil) {
            NSDictionary *menu = [NSJSONSerialization JSONObjectWithData:[json dataUsingEncoding:NSUTF8StringEncoding] options:0 error:nil];
            currentItem.title = menu[@"current"];
            quotaItem.title = menu[@"quota"];
            updatedItem.title = menu[@"updated"];
            statusMessageItem.title = menu[@"status"];
            statusMessageItem.hidden = [menu[@"status"] length] == 0;
            refreshItem.enabled = [menu[@"refresh_enabled"] boolValue];
            NSMenu *accounts = [[NSMenu alloc] initWithTitle:@"切换账号"];
            accounts.autoenablesItems = NO;
            for (NSDictionary *account in menu[@"accounts"]) {
                NSMenuItem *item = [accounts addItemWithTitle:account[@"title"] action:@selector(switchAccount:) keyEquivalent:@""];
                item.target = target;
                item.representedObject = account[@"id"];
                item.enabled = [account[@"enabled"] boolValue];
                item.state = [account[@"current"] boolValue] ? NSControlStateValueOn : NSControlStateValueOff;
            }
            switchItem.submenu = accounts;
            switchItem.enabled = accounts.numberOfItems > 0;
            [accounts release];
        }
        [json release];
    });
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
