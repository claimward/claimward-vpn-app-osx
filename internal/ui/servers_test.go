// SPDX-License-Identifier: BSD-3-Clause

// The servers sidebar and the settings window.

package ui

import (
	"testing"

	"github.com/claimward/claimward-vpn-client/pkg/appcore"
)

// fakeWindow stands in for the second window: it counts opens and closes.
type fakeWindow struct {
	opened, closed int
	refuse         bool
}

func (w *fakeWindow) open() (func(), bool) {
	if w.refuse {
		return nil, false
	}
	w.opened++
	return func() { w.closed++ }, true
}

func TestTheServersListIsTheConfiguredServerByHost(t *testing.T) {
	vm, _ := newVM(t, signedOut())
	if got := vm.Servers.Slice(); len(got) != 1 || got[0] != "vpn.example.org" {
		t.Fatalf("servers %v", got)
	}
	if vm.ServerIndex.Get() != 0 || vm.ServerName.Get() != "vpn.example.org" {
		t.Errorf("index %d name %q", vm.ServerIndex.Get(), vm.ServerName.Get())
	}
}

func TestAServerWithoutAHostIsNamedByItsURL(t *testing.T) {
	if got := serverLabel("not a url"); got != "not a url" {
		t.Errorf("label %q", got)
	}
	if got := serverLabel("https://vpn.example.org:8443/x"); got != "vpn.example.org:8443" {
		t.Errorf("label %q", got)
	}
}

func TestNoServerIsSaidSo(t *testing.T) {
	f := signedOut()
	f.st = appcore.Status{}
	f.cfg = appcore.Config{}
	vm, _ := newVM(t, f)
	if len(vm.Servers.Slice()) != 0 || vm.ServerName.Get() != "No server" {
		t.Errorf("servers %v name %q", vm.Servers.Slice(), vm.ServerName.Get())
	}
}

func TestAddServerOpensTheSettingsWindow(t *testing.T) {
	var fw fakeWindow
	vm, q := newVM(t, signedOut(), WithSettingsWindow(fw.open))
	if !vm.AddServer.CanExecute() {
		t.Fatal("+ is disabled")
	}
	run(vm, q, vm.AddServer.Execute)
	if fw.opened != 1 || !vm.SettingsInWindow.Get() || vm.Page.Get() != PageSettings {
		t.Fatalf("opened %d inWindow %v page %d", fw.opened, vm.SettingsInWindow.Get(), vm.Page.Get())
	}
}

func TestTheMainWindowKeepsTheStatusPageWhileSettingsHaveAWindow(t *testing.T) {
	var fw fakeWindow
	q := &Queue{}
	vm := NewViewModel(signedOut(), WithPost(q.Post), WithSettingsWindow(fw.open))
	v := NewView(vm, q)
	t.Cleanup(func() { vm.Stop(); vm.Wait(); v.Close() })
	vm.Refresh()
	v.Resize(testW, testH, 1)
	frame(t, v)
	click(t, v, v.w.settings)
	frame(t, v)
	if vm.Page.Get() != PageSettings || v.cards.page.Active != PageStatus || !visible(v.w.connect) || visible(v.w.server) {
		t.Errorf("page %d card %d", vm.Page.Get(), v.cards.page.Active)
	}
	vm.CloseSettings.Execute()
	frame(t, v)
	if v.cards.page.Active != PageStatus || fw.closed != 1 {
		t.Errorf("card %d closed %d", v.cards.page.Active, fw.closed)
	}
}

func TestSavingAndCancellingCloseTheWindowOnce(t *testing.T) {
	var fw fakeWindow
	f := signedOut()
	vm, q := newVM(t, f, WithSettingsWindow(fw.open))

	run(vm, q, vm.OpenSettings.Execute)
	run(vm, q, vm.SaveSettings.Execute)
	if len(f.saved) != 1 || fw.closed != 1 || vm.SettingsInWindow.Get() {
		t.Fatalf("save: saved %d closed %d inWindow %v", len(f.saved), fw.closed, vm.SettingsInWindow.Get())
	}

	run(vm, q, vm.OpenSettings.Execute)
	run(vm, q, vm.CloseSettings.Execute)
	if fw.opened != 2 || fw.closed != 2 || vm.SettingsInWindow.Get() || vm.Page.Get() != PageStatus {
		t.Fatalf("cancel: opened %d closed %d inWindow %v page %d", fw.opened, fw.closed, vm.SettingsInWindow.Get(), vm.Page.Get())
	}
	// Leaving the page again does not close a window that is gone.
	vm.Page.Set(PageStatus)
	if fw.closed != 2 {
		t.Errorf("closed %d", fw.closed)
	}
}

func TestClosingTheSettingsWindowCancelsTheForm(t *testing.T) {
	var fw fakeWindow
	vm, q := newVM(t, signedOut(), WithSettingsWindow(fw.open))
	vm.SettingsWindowClosed() // no window: nothing happens
	run(vm, q, vm.OpenSettings.Execute)
	vm.SettingsWindowClosed()
	q.Drain()
	if vm.Page.Get() != PageStatus || vm.SettingsInWindow.Get() {
		t.Errorf("page %d inWindow %v", vm.Page.Get(), vm.SettingsInWindow.Get())
	}
	// The person closed it: the app must not close it again.
	if fw.closed != 0 {
		t.Errorf("closed %d a window the person closed", fw.closed)
	}
	vm.SettingsWindowClosed()
}

func TestAWindowThatCannotOpenKeepsTheInPageForm(t *testing.T) {
	fw := fakeWindow{refuse: true}
	vm, q := newVM(t, signedOut(), WithSettingsWindow(fw.open))
	run(vm, q, vm.AddServer.Execute)
	if vm.Page.Get() != PageSettings || vm.SettingsInWindow.Get() {
		t.Errorf("page %d inWindow %v", vm.Page.Get(), vm.SettingsInWindow.Get())
	}
	// No window was closed by the person: the in-page form stays open.
	vm.SettingsWindowClosed()
	settle(vm, q)
	if vm.Page.Get() != PageSettings {
		t.Error("a window-closed event cancelled the in-page form")
	}
	run(vm, q, vm.CloseSettings.Execute)
	if vm.Page.Get() != PageStatus {
		t.Errorf("page %d", vm.Page.Get())
	}
}

func TestRemovingAServerIsNotOfferedYet(t *testing.T) {
	vm, _ := newVM(t, signedOut())
	if vm.RemoveServer.CanExecute() {
		t.Error("− is enabled")
	}
	vm.RemoveServer.Execute()
	if len(vm.Servers.Slice()) != 1 {
		t.Error("− removed the only server")
	}
}

func TestTheSettingsWindowShowsTheFormAlone(t *testing.T) {
	q := &Queue{}
	vm := NewViewModel(signedOut(), WithPost(q.Post))
	sv := NewSettingsView(vm, q)
	t.Cleanup(func() { vm.Stop(); vm.Wait(); sv.Close() })
	vm.Refresh()
	vm.OpenSettings.Execute()
	sv.Resize(440, 400, 1)
	frame2 := func() { sv.Frame() }
	frame2()
	if !visible(sv.w.server) || !visible(sv.w.save) || !visible(sv.w.cancel) {
		t.Error("the form is not laid out")
	}
}

func TestALogoThatDoesNotDecodeIsAnEmptyImage(t *testing.T) {
	if img := logoFrom([]byte("not a png")); img == nil || img.Alt != "" {
		t.Fatalf("image %v", img)
	}
	if img := logo(); img.Alt != "Claimward" {
		t.Errorf("alt %q", img.Alt)
	}
}

func TestTheServerListAndItsButtonsHaveNames(t *testing.T) {
	v, _ := newTestView(t, signedOut())
	names := map[string]bool{}
	for _, e := range v.A11yElements() {
		names[e.Name] = true
	}
	if !names["Servers"] {
		t.Errorf("no accessible Servers list among %v", names)
	}
	if v.w.serverPane.AccessibleName != "Servers" {
		t.Errorf("pane named %q", v.w.serverPane.AccessibleName)
	}
}
