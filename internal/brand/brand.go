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

// MarkWhite is the mark in white on transparent (PNG, 128×128), rendered from
// claimward/brand macos/claimward-tray-white.svg with rsvg-convert. It is the
// window's logo, on the deep-teal page.
//
//go:embed claimward-mark-white-128.png
var MarkWhite []byte
