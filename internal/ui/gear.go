// Copyright (c) 2026 the claimward authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package ui

import (
	"github.com/go-widgets/painter"
	tabler "github.com/go-widgets/tabler-icons"
	"github.com/go-widgets/toolkit"
)

// gearButton is the server list's "Edit server" button: flat like + and −,
// icon-only, drawing the Tabler "settings" cog (not the toolkit's stock
// square-with-ticks) through toolkit.SVGIcon, which rasterises it once per
// size and caches it. Its name is its label, which the ViewModel keeps current.
func gearButton() *toolkit.Button {
	b := toolkit.NewButton(editServerName, nil)
	b.Flat = true
	cog := toolkit.SVGIcon(tabler.MustSVG(tabler.Settings))
	b.Icon = func(p painter.Painter, r toolkit.Rect, ink toolkit.RGBA) {
		s := r.H * 16 / 24 // the plus's box is 14/24; the cog's own padding takes the rest
		cog(p, toolkit.Rect{X: r.X + (r.W-s)/2, Y: r.Y + (r.H-s)/2, W: s, H: s}, ink)
	}
	return b
}
