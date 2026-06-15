//go:build darwin

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>

// bringToFront promotes the process to a regular foreground app and raises the
// webview window. webview_go creates the NSWindow but never activates the app,
// so a window spawned from the menu-bar agent otherwise stays behind other apps
// (or off-screen). Calling this on the UI thread once the run loop is up makes
// the dashboard reliably come to the foreground.
static void bringToFront(void *win) {
    NSApplication *app = [NSApplication sharedApplication];
    [app setActivationPolicy:NSApplicationActivationPolicyRegular];
    [app activateIgnoringOtherApps:YES];
    NSWindow *w = (NSWindow *)win;
    if (w != NULL) {
        [w center];
        [w makeKeyAndOrderFront:nil];
        [w orderFrontRegardless];
    }
}
*/
import "C"
import "unsafe"

func bringToFront(win unsafe.Pointer) {
	C.bringToFront(win)
}
