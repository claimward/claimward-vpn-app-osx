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

// The server list and the detail panel's logos come from the toolkit's widgets,
// bound to the ViewModel's documents.
func TestTheServerListAndLogoShowTheServersLogo(t *testing.T) {
	f := twoServers()
	f.logos[urlA] = []byte(circle)
	v, vm := newTestView(t, f)
	settleView(t, v, vm)
	if vm.LogoDoc() == "" || vm.RowLogo(0) == "" {
		t.Fatalf("the ViewModel holds no logo: %q %q", vm.LogoDoc(), vm.RowLogo(0))
	}
	draw := v.w.servers.ItemRenderer
	for i, item := range []string{"Alpha, Signed in", "no state"} {
		p, buf := canvas()
		draw(p, brandTheme(), toolkit.Rect{X: 0, Y: 0, W: 60, H: 44}, i, item, i == 0, toolkit.RGBA{R: 255, G: 255, B: 255, A: 255})
		if !inked(buf) {
			t.Errorf("row %d drew nothing", i)
		}
	}
	if a := v.w.serverLogo.A11y(); a.Role != toolkit.RoleImg || a.Name != "Server logo" {
		t.Errorf("logo a11y %+v", a)
	}
}
