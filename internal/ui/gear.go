// Copyright (c) 2026 the claimward authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package ui

import (
	tabler "github.com/go-widgets/tabler-icons"
	"github.com/go-widgets/toolkit"
)

// gearButton is the server list's "Edit server" button: flat like + and −,
// icon-only, drawing the Tabler "settings" cog (not the toolkit's stock
// square-with-ticks) through toolkit.SVGGlyph, which rasterises it once per
// size and caches it. Its name is its label, which the ViewModel keeps current.
func gearButton() *toolkit.Button {
	b := toolkit.NewButton(editServerName, nil)
	b.Flat = true
	// 16/24 of the button: the plus's box is 14/24, the cog's own padding takes the rest.
	b.Icon = toolkit.SVGGlyph(tabler.MustSVG(tabler.Settings), 16, 24)
	return b
}
