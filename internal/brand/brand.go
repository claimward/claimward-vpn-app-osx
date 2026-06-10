// Package brand carries Claimward brand assets used by the app.
//
// The tray icon comes from the organization's brand repo (claimward/brand,
// macos/claimward-trayTemplate.png): an 18×18 black-on-transparent macOS
// "template" image, which the system tints automatically (white on a dark menu
// bar, black on a light one).
package brand

import _ "embed"

// TrayTemplate is the macOS menu-bar template icon (PNG, 18×18).
//
//go:embed claimward-trayTemplate.png
var TrayTemplate []byte
