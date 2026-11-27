// SPDX-License-Identifier: BSD-3-Clause
//
// Text wider than the box it is shown in wraps: it is never clipped and never
// runs past the window's edge. Buttons carry their icons.

package ui

import (
	"strings"
	"testing"

	"github.com/claimward/claimward-vpn-client/pkg/appcore"
	"github.com/go-widgets/toolkit"
)

const longValue = "https://identity.example.org/realms/a-very-long-realm-name/protocol/openid-connect/auth/with/a/path/that/never/ends"

// everyTextFits checks, for what is on screen, that each wrapped label and each
// button is as tall as its text needs at its width, and inside the window.
func everyTextFits(t *testing.T, v *View) {
	t.Helper()
	inside := func(name string, r toolkit.Rect) {
		// Rows may run below the window (the panel scrolls), never past its side.
		if r.X < 0 || r.X+r.W > v.width {
			t.Errorf("%s at %+v is outside the %dx%d window", name, r, v.width, v.high)
		}
	}
	for _, l := range v.w.wrapped() {
		if r := l.Bounds(); onScreen(l) {
			inside("label "+l.Text().Get(), r)
			if need := l.HeightFor(r.W); need > r.H {
				t.Errorf("label %q needs %d tall at %d wide, has %d", l.Text().Get(), need, r.W, r.H)
			}
		}
	}
	for _, b := range v.w.buttons() {
		if r := b.Bounds(); onScreen(b) {
			inside("button "+b.Label().Get(), r)
			if need := b.HeightFor(r.W); need > r.H {
				t.Errorf("button %q needs %d tall at %d wide, has %d", b.Label().Get(), need, r.W, r.H)
			}
		}
	}
}

func TestLongValuesWrapInTheDetailPanel(t *testing.T) {
	f := twoServers()
	f.edit(urlA, func(s *appcore.ServerStatus) {
		s.Connected, s.AssignedIP, s.Interface = true, "10.80.0.7/32", "Claimward"
		s.Endpoint = longValue
		s.AllowedIPs = []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "fd00:1234:5678::/48", "fd00:9abc:def0::/48"}
	})
	v, vm := newTestView(t, f)
	settleView(t, v, vm)
	everyTextFits(t, v)
	end := v.w.values[factEndpoint]
	if !onScreen(end) {
		t.Fatal("the endpoint row is not shown")
	}
	if r := end.Bounds(); r.H <= toolkit.Scaled(factH) {
		t.Errorf("a %d-character endpoint is on one row (%d tall)", len(longValue), r.H)
	}
	if end.Text().Get() != longValue {
		t.Errorf("the endpoint was cut: %q", end.Text().Get())
	}
}

func TestAWindowTooNarrowForTheKeysStillFits(t *testing.T) {
	f := twoServers()
	f.edit(urlA, func(s *appcore.ServerStatus) { s.Endpoint, s.Connected = longValue, true })
	q := &Queue{}
	vm := NewViewModel(f, WithPost(q.Post))
	v := NewView(vm, q)
	t.Cleanup(func() { vm.Stop(); vm.Wait(); v.Close() })
	vm.Refresh()
	v.Resize(520, 700, 1)
	for {
		vm.Wait()
		v.Frame()
		if !v.queue.Pending() {
			break
		}
	}
	everyTextFits(t, v)
}

func TestTheLogWrapsInsteadOfRunningPastItsEdge(t *testing.T) {
	f := twoServers()
	f.st.Log = []string{"short", longValue + " " + longValue}
	v, vm := newTestView(t, f)
	settleView(t, v, vm)
	rows := vm.LogRows.Slice()
	if len(rows) <= 2 {
		t.Fatalf("the log has %d rows for two lines, one of them very long: %q", len(rows), rows)
	}
	font := toolkit.CurrentFont()
	for _, r := range rows {
		if font.Measure(r) > logWidth(v.width-2*toolkit.Scaled(margin)) && len([]rune(r)) > 1 {
			t.Errorf("log row %q is wider than the log", r)
		}
	}
	if strings.Join(rows[1:], "") == "" {
		t.Error("the long line was lost")
	}
}

func TestACardThatIsNotShowingTakesNoRoom(t *testing.T) {
	for active, want := range map[int]int{PanelHidden: 0, PanelShown: 90, 2: 0} {
		if got := cardHeight(active, PanelShown, 90); got != want {
			t.Errorf("card %d takes %d, want %d", active, got, want)
		}
	}
}

func TestASectionWithNothingToShowHasNoTitle(t *testing.T) {
	if anyShown(0, factEndpoint, factTransfer) || anyShown(1<<factStatus, factEndpoint, factTransfer) {
		t.Error("an empty section reports a fact")
	}
	if !anyShown(1<<factTransfer, factEndpoint, factTransfer) || !anyShown(1<<factEndpoint, factEndpoint, factTransfer) {
		t.Error("a section with a fact at either end reports none")
	}
	// In the window: no Peer title while disconnected, one when connected.
	f := twoServers()
	v, vm := newTestView(t, f)
	if onScreen(v.w.peerTitle) || !onScreen(v.w.ifaceTitle) {
		t.Errorf("disconnected: peer title %+v interface title %+v", v.w.peerTitle.Bounds(), v.w.ifaceTitle.Bounds())
	}
	f.edit(urlA, func(s *appcore.ServerStatus) { s.Connected, s.Endpoint = true, "vpn.example.org:51820" })
	vm.Refresh()
	settleView(t, v, vm)
	if !onScreen(v.w.peerTitle) {
		t.Error("connected: no Peer title over the endpoint")
	}
}

func TestEveryButtonHasAnIcon(t *testing.T) {
	v, _ := newTestView(t, twoServers())
	for _, b := range v.w.buttons() {
		if b.LeadingIcon().Get() == nil {
			t.Errorf("button %q has no icon", b.Label().Get())
		}
	}
}

func TestTheServerRowsGrowToHoldAWrappedName(t *testing.T) {
	short := serverRowPx([]string{"A, Signed in"}, toolkit.Scaled(sidebarW))
	long := serverRowPx([]string{longValue + ", Signed in"}, toolkit.Scaled(sidebarW))
	if short < toolkit.Scaled(serverRowH) || long <= short || long > textBlock(maxRowLines)+6*toolkit.Scaled(4) {
		t.Errorf("row heights %d (short) and %d (long)", short, long)
	}
	// ListBox.RowHeight is logical: drawn (scaled) it holds the device height, at
	// every scale, and the one-pixel-per-point case is the identity.
	old := toolkit.MetricScale()
	t.Cleanup(func() { toolkit.SetMetricScale(old) })
	for _, s := range []float64{1, 1.5, 2, 3} {
		toolkit.SetMetricScale(s)
		px := serverRowPx([]string{longValue + ", Signed in"}, toolkit.Scaled(sidebarW))
		if got := toolkit.Scaled(serverRowHeight([]string{longValue + ", Signed in"}, toolkit.Scaled(sidebarW))); got < px || got > px+int(s)+1 {
			t.Errorf("at %vx the row draws %d px for %d needed", s, got, px)
		}
	}
	toolkit.SetMetricScale(1)
	if logical(40) != 40 {
		t.Errorf("logical(40) at 1x = %d", logical(40))
	}
	if textBlock(0) != 0 {
		t.Error("no lines take no room")
	}
}
