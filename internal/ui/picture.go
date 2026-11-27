// SPDX-License-Identifier: BSD-3-Clause
//
// The pictures of the detail panel and of the server list: a server's logo,
// and the status row's icon. They draw through the toolkit's own icon
// functions (toolkit.SVGIcon rasterises an SVG once per size and caches it),
// never through the painter: no drawing primitive is called here.

package ui

import (
	"strings"

	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

// drawLogo paints a server's logo into r, or a neutral mark when the server has
// none. The client already checked the SVG (same origin, digest, sanitised).
func drawLogo(p painter.Painter, r toolkit.Rect, doc string, ink toolkit.RGBA) {
	if doc == "" {
		toolkit.DrawIconApp(p, r, ink)
		return
	}
	toolkit.SVGIcon(doc)(p, r, ink)
}

// logoView is a server's logo as a widget. doc says the SVG to show now.
type logoView struct {
	toolkit.Base
	name string
	doc  func() string
}

func newLogoView(name string, doc func() string) *logoView { return &logoView{name: name, doc: doc} }

// Draw paints the logo, square, at the left of its bounds.
func (v *logoView) Draw(p painter.Painter, theme *toolkit.Theme) {
	r := v.Bounds()
	side := min(r.W, r.H)
	drawLogo(p, toolkit.Rect{X: r.X, Y: r.Y + (r.H-side)/2, W: side, H: side}, v.doc(), theme.OnSurface)
}

// A11y names the picture for the accessibility layer.
func (v *logoView) A11y() toolkit.A11yInfo {
	return toolkit.A11yInfo{Role: toolkit.RoleImg, Name: v.name}
}

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

// serverRow draws one row of the server list: the logo, the name, and under it
// the state ("Connected", "Signed in", "Signed out").
func serverRow(vm *ViewModel) func(p painter.Painter, theme *toolkit.Theme, rc toolkit.Rect, i int, item string, selected bool, ink toolkit.RGBA) {
	return func(p painter.Painter, _ *toolkit.Theme, rc toolkit.Rect, i int, item string, _ bool, ink toolkit.RGBA) {
		name, state := splitRow(item)
		pad := toolkit.Scaled(4)
		side := rc.H - 2*pad
		drawLogo(p, toolkit.Rect{X: rc.X + pad, Y: rc.Y + pad, W: side, H: side}, vm.RowLogo(i), ink)
		x := rc.X + 2*pad + side
		h := toolkit.GlyphHeight()
		toolkit.DrawText(p, x, rc.Y+(rc.H-2*h-pad/2)/2, name, ink)
		toolkit.DrawText(p, x, rc.Y+(rc.H-2*h-pad/2)/2+h+pad/2, state, ink)
	}
}

// splitRow undoes rowText.
func splitRow(item string) (name, state string) {
	if i := strings.LastIndex(item, ", "); i >= 0 {
		return item[:i], item[i+2:]
	}
	return item, ""
}
