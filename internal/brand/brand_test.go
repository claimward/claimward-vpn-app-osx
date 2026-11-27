// SPDX-License-Identifier: BSD-3-Clause

package brand

import (
	"bytes"
	"image/png"
	"testing"
)

// The tray draws the template 16 pt tall at 1x and 2x: the source must hold at
// least 64 px, and be black on transparent, or the system cannot tint it.
func TestTrayTemplateIsALargeBlackOnTransparentPNG(t *testing.T) {
	img, err := png.Decode(bytes.NewReader(TrayTemplate))
	if err != nil {
		t.Fatal(err)
	}
	b := img.Bounds()
	if b.Dx() < 64 || b.Dy() < 64 {
		t.Errorf("tray template is %dx%d, want at least 64x64", b.Dx(), b.Dy())
	}
	inked, clear := 0, 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, a := img.At(x, y).RGBA()
			switch {
			case a == 0:
				clear++
			case a == 0xffff && (r|g|bl) != 0:
				t.Fatalf("an opaque pixel at (%d,%d) is not black", x, y)
			default:
				inked++
			}
		}
	}
	if inked == 0 || clear == 0 {
		t.Errorf("%d inked, %d transparent pixels: not a glyph on transparent", inked, clear)
	}
}

func TestTheWindowMarkIsEmbedded(t *testing.T) {
	if _, err := png.Decode(bytes.NewReader(MarkWhite)); err != nil {
		t.Fatal(err)
	}
}
