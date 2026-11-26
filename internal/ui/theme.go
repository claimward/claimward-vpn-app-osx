// SPDX-License-Identifier: BSD-3-Clause
//
// The window's look: the brand's deep teal page with white ink, the same in
// the system's light and dark modes, and the white mark as its logo.

package ui

import (
	"bytes"
	"image"
	"image/draw"
	"image/png"

	"github.com/claimward/claimward-vpn-app-osx/internal/brand"
	"github.com/go-widgets/toolkit"
)

// The brand's colours (claimward/brand README, "Colours").
var (
	// deepTeal is the wordmark's "ward", #134E4A: the window's page.
	deepTeal = toolkit.RGBA{R: 0x13, G: 0x4E, B: 0x4A, A: 0xFF}
	// white is the knockout colour, #FFFFFF.
	white = toolkit.RGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}
)

// brandTheme is the deep-teal page (deepTeal) with white ink. Widget
// bodies -- buttons, the tenant list's rows -- are a lighter teal, so they
// read as raised from the page; the selected row is white with deep-teal ink.
func brandTheme() *toolkit.Theme {
	return &toolkit.Theme{
		Background:   deepTeal,
		Surface:      toolkit.RGBA{R: 0x1D, G: 0x63, B: 0x5E, A: 0xFF},
		SurfaceAlt:   toolkit.RGBA{R: 0x27, G: 0x75, B: 0x6F, A: 0xFF},
		OnBackground: white,
		OnSurface:    white,
		Accent:       white,
		Border:       toolkit.RGBA{R: 0x3B, G: 0x8A, B: 0x83, A: 0xFF},
	}
}

// logo is the white mark as RGBA pixels (straight alpha), for a toolkit.Image.
// An asset that does not decode gives an empty image rather than no window.
func logo() *toolkit.Image { return logoFrom(brand.MarkWhite) }

// logoFrom decodes a PNG into the logo.
func logoFrom(data []byte) *toolkit.Image {
	src, err := png.Decode(bytes.NewReader(data))
	if err == nil {
		b := src.Bounds()
		dst := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
		draw.Draw(dst, dst.Bounds(), src, b.Min, draw.Src)
		img := toolkit.NewImageFit(dst.Pix, b.Dx(), b.Dy())
		img.Alt = "Claimward"
		return img
	}
	return toolkit.NewImageFit(nil, 0, 0)
}
