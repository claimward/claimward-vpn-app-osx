// Package brand carries Claimward brand assets used by the app.
//
// The tray icon comes from the organization's brand repo (claimward/brand,
// macos/claimward-trayTemplate.png): an 18×18 black-on-transparent macOS
// "template" image, which the system tints automatically (white on a dark menu
// bar, black on a light one).
package brand

import (
	_ "embed"

	"github.com/go-widgets/toolkit"
)

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

// The brand's colours (claimward/brand README, "Colours").
var (
	// Teal is the brand colour, #0D9488.
	Teal = toolkit.RGBA{R: 0x0D, G: 0x94, B: 0x88, A: 0xFF}
	// DeepTeal is the wordmark's "ward", #134E4A: the window's page.
	DeepTeal = toolkit.RGBA{R: 0x13, G: 0x4E, B: 0x4A, A: 0xFF}
	// White is the knockout colour, #FFFFFF.
	White = toolkit.RGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}
)
