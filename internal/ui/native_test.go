// SPDX-License-Identifier: BSD-3-Clause

package ui

import (
	"testing"

	"github.com/go-widgets/toolkit"

	"github.com/claimward/claimward-vpn-client/pkg/hproto"
)

func keys(cs []toolkit.NativeControl) []string {
	var out []string
	for _, c := range cs {
		if c.Visible {
			out = append(out, c.Key)
		}
	}
	return out
}

func control(t *testing.T, cs []toolkit.NativeControl, key string) toolkit.NativeControl {
	t.Helper()
	for _, c := range cs {
		if c.Key == key {
			return c
		}
	}
	t.Fatalf("no native control %q in %d", key, len(cs))
	return toolkit.NativeControl{}
}

func same(a []string, b ...string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Every input is one native control; the ones of the panels that are not shown
// are still described, marked not Visible, so the host keeps its control hidden
// and finds it again by Key.
func TestNativeControlsAreOneControlPerInput(t *testing.T) {
	f := signedIn()
	f.offered = []hproto.Tenant{{ID: "chem"}, {ID: "hpc"}}
	v, vm := newTestView(t, f)
	cs := v.NativeControls()
	if len(cs) != 2 || len(keys(cs)) != 0 {
		t.Fatalf("status page: %d controls, shown %v", len(cs), keys(cs))
	}
	for _, c := range cs {
		want := toolkit.NativeEntry
		if c.Key == "provider" || c.Key == "tenant" {
			want = toolkit.NativePopUp
		}
		if c.Kind != want {
			t.Errorf("%q: kind %v, want %v", c.Key, c.Kind, want)
		}
	}
	// The tenant pop-up is shown once the server asks for a choice.
	clickAndSettle(t, v, vm, v.w.connect)
	cs = v.NativeControls()
	if got := keys(cs); !same(got, "tenant") {
		t.Fatalf("tenant asked: %v", got)
	}
	if c := control(t, cs, "tenant"); c.Rect.W <= 0 || c.Rect.H <= 0 {
		t.Errorf("visible but unplaced %+v", c.Rect)
	}
	control(t, cs, "tenant").OnText("hpc")
	settleView(t, v, vm)
	if c := f.last("SetTenant"); c.arg != "hpc" {
		t.Fatalf("SetTenant %+v", c)
	}

	// The address form shows its entry; editing shows the provider pop-up.
	vm.AddServer.Execute()
	settleView(t, v, vm)
	if got := keys(v.NativeControls()); !same(got, "server_url") {
		t.Fatalf("add form: %v", got)
	}
}

func TestNativeTextReachesTheViewModelAndBack(t *testing.T) {
	v, vm := newTestView(t, signedIn())
	vm.AddServer.Execute()
	settleView(t, v, vm)
	control(t, v.NativeControls(), "server_url").OnText("https://other.example.org")
	if got := vm.ServerURL.Get(); got != "https://other.example.org" {
		t.Fatalf("view model server %q", got)
	}
	if got := v.w.server.Text().Get(); got != "https://other.example.org" {
		t.Fatalf("fallback entry %q", got)
	}
	vm.ServerURL.Set("https://third.example.org")
	if got := control(t, v.NativeControls(), "server_url").Text; got != "https://third.example.org" {
		t.Fatalf("native control %q", got)
	}
	if !v.NeedsPresent() {
		t.Error("text typed in the native control did not ask for a repaint")
	}
}

// Choosing a provider in the pop-up chooses it, and the pop-up follows back.
func TestNativeProviderPopUpChoosesTheProvider(t *testing.T) {
	f := twoServers()
	v, vm := newTestView(t, f)
	vm.ServerIndex.Set(1)
	settleView(t, v, vm)
	clickAndSettle(t, v, vm, v.w.edit)
	cs := v.NativeControls()
	if got := keys(cs); !same(got, "provider") {
		t.Fatalf("edit form: %v of %+v (card %d page %d)", got, cs, vm.FormCard.Get(), vm.PageShown.Get())
	}
	if got := control(t, cs, "provider").Items; !same(got, "GitHub", "Campus") {
		t.Fatalf("provider items %v", got)
	}
	control(t, cs, "provider").OnText("Campus")
	settleView(t, v, vm)
	if c := f.last("SetProvider"); c.server != urlB || c.arg != "campus" {
		t.Fatalf("SetProvider %+v", c)
	}
	if got := control(t, v.NativeControls(), "provider").Text; got != "Campus" {
		t.Fatalf("pop-up shows %q", got)
	}
}

func TestClaimingStandsTheFallbackDown(t *testing.T) {
	v, vm := newTestView(t, signedIn())
	vm.AddServer.Execute()
	settleView(t, v, vm)
	control(t, v.NativeControls(), "server_url").OnClaim(true)
	if !v.w.nServer.Claimed().Get() {
		t.Fatal("OnClaim did not claim the region")
	}
	if !frame(t, v) {
		t.Fatal("claiming did not ask for a repaint")
	}
}

func TestNativeControlsBeforeTheFirstLayoutIsEmpty(t *testing.T) {
	v := NewView(NewViewModel(signedOut()), &Queue{})
	t.Cleanup(v.Close)
	if cs := v.NativeControls(); cs != nil {
		t.Fatalf("controls before any layout: %v", cs)
	}
}
