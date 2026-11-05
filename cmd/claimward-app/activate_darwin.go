//go:build darwin

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>

// raiseWindow activates the app and brings the given window to the front.
static void raiseWindow(NSWindow *w) {
    NSApplication *app = [NSApplication sharedApplication];
    [app activateIgnoringOtherApps:YES];
    if (w != NULL) {
        [w makeKeyAndOrderFront:nil];
        [w orderFrontRegardless];
    }
}

// bringToFront promotes the process to a regular foreground app and raises the
// webview window. webview_go creates the NSWindow but never activates the app,
// so a window spawned from the menu-bar agent otherwise stays behind other apps
// (or off-screen). Calling this on the UI thread once the run loop is up makes
// the dashboard reliably come to the foreground.
static void bringToFront(void *win) {
    NSApplication *app = [NSApplication sharedApplication];
    [app setActivationPolicy:NSApplicationActivationPolicyRegular];
    NSWindow *w = (NSWindow *)win;
    if (w != NULL) {
        [w center];
    }
    raiseWindow(w);
}

// keepWindowFront re-raises the window whenever the app becomes active. The tray
// re-opens an already-running dashboard with `open`, which activates the app but
// does not re-order the existing webview window — so without this the window
// stays behind other windows on a second "Open Claimward…". Observing
// NSApplicationDidBecomeActiveNotification raises it on every activation (open,
// Dock click, Cmd-Tab). The observer lives for the app's lifetime.
static void keepWindowFront(void *win) {
    NSWindow *w = (NSWindow *)win;
    if (w == NULL) {
        return;
    }
    [[NSNotificationCenter defaultCenter]
        addObserverForName:NSApplicationDidBecomeActiveNotification
                    object:nil
                     queue:[NSOperationQueue mainQueue]
                usingBlock:^(NSNotification *note) {
                    [w makeKeyAndOrderFront:nil];
                    [w orderFrontRegardless];
                }];
}
*/
import "C"
import "unsafe"

func bringToFront(win unsafe.Pointer) {
	C.bringToFront(win)
}

// keepWindowFront installs an observer that re-raises the window on every app
// activation, so re-opening an already-running dashboard brings it to the front.
func keepWindowFront(win unsafe.Pointer) {
	C.keepWindowFront(win)
}
