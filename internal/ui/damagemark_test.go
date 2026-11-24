// SPDX-License-Identifier: BSD-3-Clause

package ui

import (
	"testing"

	"github.com/go-widgets/toolkit"
)

// A widget wholly outside the frame has nothing to repaint.
func TestMarkingAWidgetOutsideTheFrameDamagesNothing(t *testing.T) {
	v, _ := newTestView(t, signedOut())
	frame(t, v)
	v.DamageRects()
	l := toolkit.NewLabel("x")
	l.SetBounds(toolkit.Rect{X: -40, Y: -40, W: 10, H: 10})
	v.mark(l)
	if got := v.DamageRects(); len(got) != 0 {
		t.Fatalf("damage %v for a widget off the frame", got)
	}
}

// Once the whole frame is to be reported, a single widget's rectangle is not
// kept as well.
func TestMarkingWhileTheWholeFrameIsDamagedKeepsNoRectangle(t *testing.T) {
	v, _ := newTestView(t, signedOut())
	frame(t, v)
	v.wholeFrame()
	l := toolkit.NewLabel("x")
	l.SetBounds(toolkit.Rect{X: 10, Y: 10, W: 20, H: 20})
	v.mark(l)
	if len(v.damage) != 0 {
		t.Fatalf("%d rectangles kept beside a whole-frame damage", len(v.damage))
	}
	if got := v.DamageRects(); got != nil {
		t.Fatalf("whole frame reported as %v", got)
	}
}
