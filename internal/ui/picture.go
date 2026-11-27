// SPDX-License-Identifier: BSD-3-Clause
//
// The status row's icon. A server's logo, and the server list's rows, come
// from the toolkit (toolkit.Logo, toolkit.LogoRows): nothing here draws an SVG
// itself, and no drawing primitive is called but the toolkit's own icon dot.

package ui

import (
	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

// The status icon's inks: up, idle, problem.
var (
	inkUp      = toolkit.RGBA{R: 0x2e, G: 0xa0, B: 0x43, A: 0xff}
	inkProblem = toolkit.RGBA{R: 0xd1, G: 0x24, B: 0x2f, A: 0xff}
)

// statusIcon is the status row's dot: green when the tunnel is up, red after a
// failure, the text's ink otherwise. kind says which now (StatusIdle ...).
type statusIcon struct {
	toolkit.Base
	kind func() int
}

func newStatusIcon(kind func() int) *statusIcon { return &statusIcon{kind: kind} }

// Draw paints the dot, square, centred in its bounds.
func (s *statusIcon) Draw(p painter.Painter, theme *toolkit.Theme) {
	r := s.Bounds()
	side := min(r.W, r.H)
	ink := theme.OnSurface
	switch s.kind() {
	case StatusUp:
		ink = inkUp
	case StatusProblem:
		ink = inkProblem
	}
	toolkit.DrawIconDot(p, toolkit.Rect{X: r.X + (r.W-side)/2, Y: r.Y + (r.H-side)/2, W: side, H: side}, ink)
}

// A11y says the status in words, for the accessibility layer.
func (s *statusIcon) A11y() toolkit.A11yInfo {
	name := "Not connected"
	switch s.kind() {
	case StatusUp:
		name = "Connected"
	case StatusProblem:
		name = "Problem"
	}
	return toolkit.A11yInfo{Role: toolkit.RoleImg, Name: name}
}
