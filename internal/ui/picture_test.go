// SPDX-License-Identifier: BSD-3-Clause

package ui

import (
	"testing"

	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

const circle = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><circle cx="5" cy="5" r="4" fill="#000"/></svg>`

func canvas() (painter.Painter, []byte) {
	buf := make([]byte, 4*64*64)
	return painter.NewPixelPainter(buf, 64, 64), buf
}

func inked(buf []byte) bool {
	for _, b := range buf {
		if b != 0 {
			return true
		}
	}
	return false
}

func TestALogoWithAndWithoutADocument(t *testing.T) {
	ink := toolkit.RGBA{R: 255, G: 255, B: 255, A: 255}
	for _, doc := range []string{"", circle} {
		p, buf := canvas()
		drawLogo(p, toolkit.Rect{X: 8, Y: 8, W: 32, H: 32}, doc, ink)
		if !inked(buf) {
			t.Errorf("document %q drew nothing", doc)
		}
	}
}

func TestTheLogoViewIsSquareAndNamed(t *testing.T) {
	doc := ""
	l := newLogoView("Server logo", func() string { return doc })
	l.SetBounds(toolkit.Rect{X: 4, Y: 4, W: 50, H: 30})
	for _, d := range []string{"", circle} {
		doc = d
		p, buf := canvas()
		l.Draw(p, brandTheme())
		if !inked(buf) {
			t.Errorf("document %q drew nothing", d)
		}
	}
	if a := l.A11y(); a.Role != toolkit.RoleImg || a.Name != "Server logo" {
		t.Errorf("a11y %+v", a)
	}
}

func TestTheStatusIconIsInkedPerKind(t *testing.T) {
	kind := StatusIdle
	s := newStatusIcon(func() int { return kind })
	s.SetBounds(toolkit.Rect{X: 4, Y: 4, W: 30, H: 20})
	for _, k := range []int{StatusIdle, StatusUp, StatusProblem} {
		kind = k
		p, buf := canvas()
		s.Draw(p, brandTheme())
		if !inked(buf) {
			t.Errorf("kind %d drew nothing", k)
		}
	}
}

func TestAServerRowShowsTheLogoNameAndState(t *testing.T) {
	f := twoServers()
	f.logos[urlA] = []byte(circle)
	vm, _ := newVM(t, f)
	draw := serverRow(vm)
	for i, item := range []string{"Alpha, Signed in", "no state"} {
		p, buf := canvas()
		draw(p, brandTheme(), toolkit.Rect{X: 0, Y: 0, W: 60, H: 44}, i, item, i == 0, toolkit.RGBA{R: 255, G: 255, B: 255, A: 255})
		if !inked(buf) {
			t.Errorf("row %d drew nothing", i)
		}
	}
}
