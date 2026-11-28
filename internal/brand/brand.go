// Package brand carries Claimward brand assets used by the app.
//
// The tray icon is rendered from the organization's brand repo (claimward/brand,
// macos/claimward-tray-Template.svg): a black-on-transparent macOS "template"
// image, which the system tints automatically (white on a dark menu bar, black
// on a light one). The tray (go-widgets/tray v0.18.0) takes PNG only and draws
// it 16 pt tall with 1x and 2x representations, so the source is 64 px, never
// the 18 px export that a Retina bar would upscale.
package brand

import _ "embed"

// TrayTemplate is the macOS menu-bar template icon (PNG, 64×64), rendered with
// rsvg-convert -w 64 -h 64 from claimward-tray-Template.svg.
//
//go:embed claimward-trayTemplate.png
var TrayTemplate []byte

// MarkWhite is the mark in white on transparent (PNG, 128×128), rendered from
// claimward/brand macos/claimward-tray-white.svg with rsvg-convert. It is the
// window's logo, on the deep-teal page.
//
//go:embed claimward-mark-white-128.png
var MarkWhite []byte
