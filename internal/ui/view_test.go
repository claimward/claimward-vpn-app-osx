// SPDX-License-Identifier: BSD-3-Clause
//
// View tests: the real widget tree, bound to a ViewModel over a fake Core,
// driven through the application.Handler methods the window calls -- input
// in, pixels out -- with no window.

package ui

import (
	"testing"

	"github.com/claimward/claimward-vpn-client/pkg/appcore"
	"github.com/claimward/claimward-vpn-client/pkg/hproto"
	"github.com/go-widgets/application"
	"github.com/go-widgets/toolkit"

	"github.com/claimward/claimward-vpn-app-osx/internal/backend"
)

const testW, testH = 760, 900

// newTestView is a View over f, sized and drawn once.
func newTestView(t *testing.T, f *fakeCore, opts ...Option) (*View, *ViewModel) {
	t.Helper()
	q := &Queue{}
	vm := NewViewModel(f, append([]Option{WithPost(q.Post)}, opts...)...)
	v := NewView(vm, q)
	t.Cleanup(func() { vm.Stop(); vm.Wait(); v.Close() })
	vm.Refresh()
	v.Resize(testW, testH, 1)
	settleView(t, v, vm)
	return v, vm
}

// settleView waits for every action to finish and runs the frames that drain
// what they posted, until nothing more is posted.
func settleView(t *testing.T, v *View, vm *ViewModel) {
	t.Helper()
	for {
		vm.Wait()
		frame(t, v)
		if !v.queue.Pending() {
			return
		}
	}
}

// frame runs one tick and reports whether it drew.
func frame(t *testing.T, v *View) bool {
	t.Helper()
	buf, w, h, changed := v.Frame()
	if changed && (w != testW || h != testH || len(buf) != 4*w*h) {
		t.Fatalf("frame %dx%d with %d bytes", w, h, len(buf))
	}
	return changed
}

func click(t *testing.T, v *View, w toolkit.Widget) {
	t.Helper()
	r := w.Bounds()
	if r.W <= 0 || r.H <= 0 {
		t.Fatalf("%T is not on screen (%+v)", w, r)
	}
	// Widget bounds are in surface coordinates; so is the pointer.
	x, y := r.X+r.W/2, r.Y+r.H/2
	v.MouseDown(x, y)
	v.MouseUp(x, y)
	frame(t, v)
}

// clickAndSettle is a click whose command runs an action.
func clickAndSettle(t *testing.T, v *View, vm *ViewModel, w toolkit.Widget) {
	t.Helper()
	click(t, v, w)
	settleView(t, v, vm)
}

func onScreen(w toolkit.Widget) bool { r := w.Bounds(); return r.W > 0 && r.H > 0 }

// typeInto types s into the entry that has focus.
func typeInto(v *View, s string) {
	for _, r := range s {
		v.Key("", r)
	}
}

func TestTheWindowDrawsTheStatus(t *testing.T) {
	v, _ := newTestView(t, twoServers())
	if v.w.values[factStatus].Text().Get() != "Disconnected" || v.w.values[factAccount].Text().Get() != "Signed in as ada@example.org" {
		t.Errorf("labels %q %q", v.w.values[factStatus].Text().Get(), v.w.values[factAccount].Text().Get())
	}
	if v.w.serverName.Text().Get() != "Alpha" {
		t.Errorf("server name %q", v.w.serverName.Text().Get())
	}
	if frame(t, v) {
		t.Error("an idle window redrew")
	}
	if v.NeedsPresent() {
		t.Error("an idle window asks to be presented")
	}
	// The background is painted.
	buf, _, _, _ := v.Frame()
	if bg := v.theme.Background; buf[0] != bg.R || buf[1] != bg.G || buf[2] != bg.B {
		t.Errorf("top-left pixel %v, want %v", buf[:4], bg)
	}
	for _, w := range []toolkit.Widget{v.w.connect, v.w.disconnect, v.w.signOut, v.w.log, v.w.servers, v.w.serverPane, v.w.serverLogo, v.w.statusIcon, v.w.edit} {
		if !onScreen(w) {
			t.Errorf("%T is not laid out", w)
		}
	}
	for _, w := range []toolkit.Widget{v.w.nServer, v.w.deviceCode, v.w.tenantPick, v.w.signInWith[0], v.w.confirmRemove, v.w.previewLogo} {
		if onScreen(w) {
			t.Errorf("%T shows on the status page", w)
		}
	}
	if v.w.servers.Selected().Get() != 0 || len(v.w.servers.Items) != 2 {
		t.Errorf("servers %d selected %d", len(v.w.servers.Items), v.w.servers.Selected().Get())
	}
}

// A click on Connect runs the command, and the result reaches the labels
// through the queue, on the next frame.
func TestClickingConnect(t *testing.T) {
	f := signedIn()
	v, vm := newTestView(t, f)
	if v.w.connect.Disabled().Get() || !v.w.disconnect.Disabled().Get() {
		t.Error("Connect and Disconnect are not enabled as the state says")
	}
	clickAndSettle(t, v, vm, v.w.connect)
	if f.called("Connect") != 1 || v.w.values[factStatus].Text().Get() != "Connected" {
		t.Errorf("Connect called %d times; state %q", f.called("Connect"), v.w.values[factStatus].Text().Get())
	}
	if !v.w.connect.Disabled().Get() || v.w.disconnect.Disabled().Get() {
		t.Error("after connecting, Connect is enabled or Disconnect is not")
	}
	// A disabled button does nothing.
	click(t, v, v.w.connect)
	if f.called("Connect") != 1 {
		t.Error("a disabled Connect connected")
	}
	clickAndSettle(t, v, vm, v.w.disconnect)
	if f.called("Disconnect") != 1 || v.w.values[factStatus].Text().Get() != "Disconnected" {
		t.Errorf("Disconnect called %d times; state %q", f.called("Disconnect"), v.w.values[factStatus].Text().Get())
	}
	clickAndSettle(t, v, vm, v.w.signOut)
	if f.called("Logout") != 1 || !onScreen(v.w.signInWith[0]) {
		t.Errorf("Logout called %d times", f.called("Logout"))
	}
}

// Choosing a row of the list selects that server, and its detail follows.
func TestClickingARowSelectsTheServer(t *testing.T) {
	v, vm := newTestView(t, twoServers())
	r := v.w.servers.Bounds()
	rowH := v.w.servers.RowHeight
	x, y := r.X+r.W/2, r.Y+rowH+rowH/2
	v.MouseDown(x, y)
	v.MouseUp(x, y)
	settleView(t, v, vm)
	if vm.ServerIndex.Get() != 1 || v.w.serverName.Text().Get() != "Beta" || v.w.values[factURL].Text().Get() != urlB {
		t.Fatalf("selected %d %q", vm.ServerIndex.Get(), v.w.serverName.Text().Get())
	}
	// Beta lists two providers: one button each, and the sign-in rebuilds the panel.
	if !onScreen(v.w.signInWith[0]) || !onScreen(v.w.signInWith[1]) || onScreen(v.w.signInWith[2]) {
		t.Fatal("not one sign-in button per provider")
	}
	if v.w.signInWith[1].Label().Get() != "Sign in with Campus" {
		t.Errorf("label %q", v.w.signInWith[1].Label().Get())
	}
}

func TestSigningInFromTheWindow(t *testing.T) {
	f := signedOut()
	v, vm := newTestView(t, f)
	if !onScreen(v.w.signInWith[0]) || v.w.signInWith[0].Label().Get() != "Sign in" {
		t.Fatal("no Sign in button on a signed-out server")
	}
	release := f.hoist("Login")
	click(t, v, v.w.signInWith[0])
	f.waitFor(t, "Login", 1)
	f.edit(urlA, func(s *appcore.ServerStatus) {
		s.DeviceVerificationURI, s.DeviceUserCode = "https://github.com/login/device", "WDJB-MJHT"
	})
	vm.Refresh()
	until(t, v.queue, func() bool { return v.w.deviceCode.Text().Get() == "WDJB-MJHT" })
	frame(t, v)
	if !onScreen(v.w.deviceCode) || !onScreen(v.w.openPage) || v.w.deviceCode.Text().Get() != "WDJB-MJHT" {
		t.Fatalf("the device prompt is not shown: %+v %q", v.w.deviceCode.Bounds(), v.w.deviceCode.Text().Get())
	}
	if v.w.busy.Text().Get() == "" {
		t.Error("nothing says a sign-in is under way")
	}
	click(t, v, v.w.cancelSignIn)
	release()
	settleView(t, v, vm)
	f.edit(urlA, func(s *appcore.ServerStatus) { s.DeviceVerificationURI, s.DeviceUserCode = "", "" })
	vm.Refresh()
	settleView(t, v, vm)
	if onScreen(v.w.deviceCode) {
		t.Error("the device prompt outlives the sign-in")
	}

	// Signed in at last: the buttons go, the panel is laid out again.
	f.edit(urlA, func(s *appcore.ServerStatus) { s.LoggedIn = true })
	vm.Refresh()
	settleView(t, v, vm)
	if visible(v.w.shown, shownProvider) {
		t.Error("a Sign in button on a signed-in server")
	}
}

// The tenant choice, through the DropDown as a person does: open it, click a row.
func TestChoosingATenantInTheWindow(t *testing.T) {
	f := signedIn()
	f.offered = []hproto.Tenant{{ID: "chem", Name: "Chemistry"}, {ID: "hpc", Name: "HPC"}}
	v, vm := newTestView(t, f)
	if onScreen(v.w.tenantPick) {
		t.Fatal("the tenant choice shows before it is asked for")
	}
	clickAndSettle(t, v, vm, v.w.connect)
	if !onScreen(v.w.tenantPick) {
		t.Fatalf("the tenant choice is not shown after ErrTenantRequired: %+v", v.w.tenantPick.Bounds())
	}
	if got := v.w.message.Text().Get(); got == "" {
		t.Error("no message after ErrTenantRequired")
	}
	// Refresh asks the server for its tenants again.
	clickAndSettle(t, v, vm, v.w.chooseTenant)
	if f.called("Tenants") == 0 {
		t.Error("Refresh did not ask for the tenants")
	}
	click(t, v, v.w.tenantPick)
	if !v.w.tenantPick.Open().Get() {
		t.Fatal("the DropDown did not open")
	}
	pb := v.w.tenantPick.PopoverBounds()
	row := pb.H / 3
	v.MouseDown(pb.X+pb.W/2, pb.Y+2*row+row/2) // the third row: HPC
	v.MouseUp(pb.X+pb.W/2, pb.Y+2*row+row/2)
	settleView(t, v, vm)
	if c := f.last("SetTenant"); c.server != urlA || c.arg != "hpc" {
		t.Fatalf("SetTenant %+v", c)
	}
	clickAndSettle(t, v, vm, v.w.connect)
	if v.w.values[factTenant].Text().Get() != "Tenant: HPC (hpc)" || visible(v.w.shown, shownTenant) {
		t.Errorf("tenant line %q", v.w.values[factTenant].Text().Get())
	}
}

func TestAddingAServerFromTheWindow(t *testing.T) {
	f := signedIn()
	f.previews[urlC] = backend.Preview{Name: "Gamma"}
	v, vm := newTestView(t, f)
	clickAndSettle(t, v, vm, v.w.serverPane.Add)
	if vm.Page.Get() != PageSettings || !onScreen(v.w.nServer) || onScreen(v.w.connect) {
		t.Fatalf("page %d; server field %+v", vm.Page.Get(), v.w.nServer.Bounds())
	}
	click(t, v, v.w.server)
	typeInto(v, "https://c.example.org")
	v.Key("Backspace", 0)
	v.Key("Left", 0)
	v.Key("Right", 0)
	v.Key("", 'g')
	frame(t, v)
	if vm.ServerURL.Get() != urlC {
		t.Fatalf("typed %q", vm.ServerURL.Get())
	}
	// Ctrl+V pastes into the field that has focus; no modifier is no shortcut.
	toolkit.SetClipboardText(urlC)
	v.Key("Backspace", 0)
	vm.ServerURL.Set("")
	frame(t, v)
	click(t, v, v.w.server)
	v.Shortcut('v', true, false)
	v.Shortcut('v', false, false)
	frame(t, v)
	if vm.ServerURL.Get() != urlC {
		t.Fatalf("pasted %q", vm.ServerURL.Get())
	}
	clickAndSettle(t, v, vm, v.w.next)
	if vm.FormCard.Get() != FormPreview || !onScreen(v.w.previewLogo) || v.w.previewName.Text().Get() != "Gamma" {
		t.Fatalf("card %d name %q", vm.FormCard.Get(), v.w.previewName.Text().Get())
	}
	clickAndSettle(t, v, vm, v.w.back)
	if vm.FormCard.Get() != FormURL {
		t.Fatalf("card %d after Back", vm.FormCard.Get())
	}
	clickAndSettle(t, v, vm, v.w.cancel)
	if vm.Page.Get() != PageStatus || !onScreen(v.w.connect) {
		t.Fatal("Cancel did not return to the servers")
	}

	clickAndSettle(t, v, vm, v.w.serverPane.Add)
	click(t, v, v.w.server)
	typeInto(v, urlC)
	clickAndSettle(t, v, vm, v.w.next)
	clickAndSettle(t, v, vm, v.w.cancelPreview)
	if vm.Page.Get() != PageStatus {
		t.Fatal("Cancel on the preview did not return to the servers")
	}

	clickAndSettle(t, v, vm, v.w.serverPane.Add)
	click(t, v, v.w.server)
	typeInto(v, urlC)
	clickAndSettle(t, v, vm, v.w.next)
	clickAndSettle(t, v, vm, v.w.add)
	if f.last("AddServer").server != urlC || vm.Page.Get() != PageStatus || len(v.w.servers.Items) != 2 {
		t.Fatalf("AddServer %+v page %d rows %d", f.last("AddServer"), vm.Page.Get(), len(v.w.servers.Items))
	}
}

func TestEditingAServerFromTheWindow(t *testing.T) {
	f := twoServers()
	v, vm := newTestView(t, f)
	r := v.w.servers.Bounds()
	h := v.w.servers.RowHeight
	v.MouseDown(r.X+r.W/2, r.Y+h+h/2)
	v.MouseUp(r.X+r.W/2, r.Y+h+h/2)
	settleView(t, v, vm)
	clickAndSettle(t, v, vm, v.w.edit)
	if vm.FormCard.Get() != FormEdit || !onScreen(v.w.provider) || v.w.editURL.Text().Get() != urlB {
		t.Fatalf("card %d url %q", vm.FormCard.Get(), v.w.editURL.Text().Get())
	}
	click(t, v, v.w.provider)
	pb := v.w.provider.PopoverBounds()
	row := pb.H / 2
	v.MouseDown(pb.X+pb.W/2, pb.Y+row+row/2) // the second row: Campus
	v.MouseUp(pb.X+pb.W/2, pb.Y+row+row/2)
	settleView(t, v, vm)
	if c := f.last("SetProvider"); c.server != urlB || c.arg != "campus" {
		t.Fatalf("SetProvider %+v", c)
	}
	clickAndSettle(t, v, vm, v.w.done)
	if vm.Page.Get() != PageStatus {
		t.Fatal("Done did not return to the servers")
	}
	// The gear opens the same form.
	clickAndSettle(t, v, vm, v.w.serverPane.Extra[0])
	if vm.FormCard.Get() != FormEdit {
		t.Fatal("the gear did not edit the server")
	}
}

func TestRemovingAServerFromTheWindow(t *testing.T) {
	f := twoServers()
	f.approved[urlA] = true
	v, vm := newTestView(t, f)
	if onScreen(v.w.confirmRemove) {
		t.Fatal("the removal question shows by itself")
	}
	clickAndSettle(t, v, vm, v.w.serverPane.Remove)
	if !onScreen(v.w.confirmRemove) || v.w.removeText.Text().Get() != "Remove Alpha?" {
		t.Fatalf("removal card %+v %q", v.w.confirmRemove.Bounds(), v.w.removeText.Text().Get())
	}
	clickAndSettle(t, v, vm, v.w.cancelRemove)
	if onScreen(v.w.confirmRemove) || f.called("RemoveServer") != 0 {
		t.Fatal("Cancel removed the server")
	}
	clickAndSettle(t, v, vm, v.w.serverPane.Remove)
	clickAndSettle(t, v, vm, v.w.confirmRevoke)
	if f.called("Revoke") != 1 || f.last("RemoveServer").server != urlA || len(v.w.servers.Items) != 1 {
		t.Fatalf("revoke %d removed %+v rows %d", f.called("Revoke"), f.last("RemoveServer"), len(v.w.servers.Items))
	}
	clickAndSettle(t, v, vm, v.w.serverPane.Remove)
	clickAndSettle(t, v, vm, v.w.confirmRemove)
	if len(v.w.servers.Items) != 0 || v.w.serverName.Text().Get() != "No server" {
		t.Errorf("rows %d name %q", len(v.w.servers.Items), v.w.serverName.Text().Get())
	}
}

func TestTheServerWindowIsTheFormAlone(t *testing.T) {
	f := signedIn()
	q := &Queue{}
	var sv *View
	vm := NewViewModel(f, WithPost(q.Post), WithSettingsWindow(func() (func(), bool) { return func() {}, true }))
	v := NewView(vm, q)
	sv = NewSettingsView(vm, q)
	t.Cleanup(func() { vm.Stop(); vm.Wait(); v.Close(); sv.Close() })
	vm.Refresh()
	v.Resize(testW, testH, 1)
	sv.Resize(testW, testH, 1)
	settleView(t, v, vm)
	frame(t, sv)
	vm.AddServer.Execute()
	settleView(t, v, vm)
	frame(t, sv)
	if !onScreen(sv.w.nServer) || !onScreen(sv.w.next) || v.vm.PageShown.Get() != PageStatus {
		t.Fatalf("form %+v", sv.w.nServer.Bounds())
	}
}

func TestTheLogScrolls(t *testing.T) {
	f := signedIn()
	for i := 0; i < 200; i++ {
		f.st.Log = append(f.st.Log, "10:00:00  line")
	}
	v, _ := newTestView(t, f)
	if len(v.w.log.Items) != 200 {
		t.Fatalf("log rows %d", len(v.w.log.Items))
	}
	r := v.w.log.Bounds()
	v.MouseMove(r.X+10, r.Y+10)
	v.Scroll(120)
	v.Scroll(5)
	v.Scroll(-5)
	v.Scroll(0)
	if !frame(t, v) {
		t.Error("scrolling did not redraw")
	}
}

func TestAppearanceAndScale(t *testing.T) {
	v, _ := newTestView(t, signedOut())
	// The brand page is deep teal in light and dark alike.
	want := v.theme.Background
	v.SystemAppearance(application.SystemAppearance{Dark: true})
	v.SystemAppearance(application.SystemAppearance{})
	if frame(t, v) || v.theme.Background != want {
		t.Error("the system appearance changed the brand page")
	}
	// A display of another density rebuilds the boxes at its scale.
	old := toolkit.MetricScale()
	t.Cleanup(func() { toolkit.SetMetricScale(old) })
	h := v.w.connect.Bounds().H
	toolkit.SetMetricScale(2)
	v.Resize(testW*2, testH*2, 2)
	v.Frame()
	if got := v.w.connect.Bounds().H; got != 2*h {
		t.Errorf("at scale 2 the button is %d high, want %d", got, 2*h)
	}
}

func TestAWindowNotYetSized(t *testing.T) {
	q := &Queue{}
	vm := NewViewModel(signedOut(), WithPost(q.Post))
	v := NewView(vm, q)
	defer v.Close()
	if buf, _, _, changed := v.Frame(); buf != nil || changed {
		t.Error("an unsized window drew")
	}
	v.Resize(0, 10, 1) // ignored
	v.MouseDown(1, 1)  // no tree yet: ignored
	if buf, _, _, _ := v.Frame(); buf != nil {
		t.Error("a zero-size resize allocated")
	}
	if !v.NeedsPresent() || !v.PresentImmediate() || v.PresentThrottle() {
		t.Error("present gate")
	}
}

func TestQueueRunsWhatItsFunctionsPost(t *testing.T) {
	q := &Queue{}
	var got []int
	q.Post(func() { got = append(got, 1); q.Post(func() { got = append(got, 3) }) })
	q.Post(func() { got = append(got, 2) })
	if !q.Pending() {
		t.Error("nothing pending")
	}
	q.Drain()
	if len(got) != 3 || got[0] != 1 || got[1] != 2 || got[2] != 3 || q.Pending() {
		t.Errorf("ran %v", got)
	}
}

// A move with the button held is a drag (a scrollbar thumb), not a hover.
func TestADragIsADrag(t *testing.T) {
	v, _ := newTestView(t, signedIn())
	r := v.w.log.Bounds()
	v.MouseDown(r.X+r.W-3, r.Y+5)
	v.MouseMove(r.X+r.W-3, r.Y+40)
	v.MouseUp(r.X+r.W-3, r.Y+40)
	v.MouseMove(r.X+5, r.Y+5)
	if v.pressed {
		t.Error("still pressed after the release")
	}
}

func TestUseOpenTypeText(t *testing.T) {
	if err := UseOpenTypeText(); err != nil {
		t.Fatal(err)
	}
}
