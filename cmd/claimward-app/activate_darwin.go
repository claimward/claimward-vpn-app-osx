// SPDX-License-Identifier: BSD-3-Clause

//go:build darwin

package main

import "github.com/go-macos/objc"

// activate brings the application, and with it its window, to the front.
func activate() {
	objc.App().Send(objc.Sel("activateIgnoringOtherApps:"), true)
}
