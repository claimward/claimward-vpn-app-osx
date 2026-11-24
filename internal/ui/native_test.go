// SPDX-License-Identifier: BSD-3-Clause

package ui

import (
	"testing"

	"github.com/go-widgets/toolkit"

	"github.com/claimward/claimward-vpn-client/pkg/appcore"
)

// settingsControls opens the settings page and returns what the host embeds.
func settingsControls(t *testing.T, provider string) (*View, *ViewModel, []toolkit.NativeControl) {
	t.Helper()
	f := signedOut()
	f.cfg = appcore.Config{Provider: provider}
	v, vm := newTestView(t, f)
	click(t, v, v.w.settings)
	frame(t, v)
	return v, vm, v.NativeControls()
}

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

// Every input on the page being shown is one native control: a text field for
// each text input, a pop-up button for the identity provider. The controls of
// the panels that are not shown are still described, marked not Visible, so
// the host keeps its control hidden and finds it again by Key.
func TestNativeControlsAreOneControlPerShownInput(t *testing.T) {
	_, _, cs := settingsControls(t, "github")
	if got := keys(cs); !same(got, "server_url", "provider", "github_client_id") {
		t.Fatalf("github provider: native controls %v", got)
	}
	// Four text inputs, the provider pop-up and the tenant pop-up.
	if len(cs) != 6 {
		t.Fatalf("%d native controls described, want one per input (6)", len(cs))
	}
	for _, c := range cs {
		want := toolkit.NativeEntry
		if c.Key == "provider" || c.Key == "tenant" {
			want = toolkit.NativePopUp
		}
		if c.Kind != want {
			t.Fatalf("%q: kind %v, want %v", c.Key, c.Kind, want)
		}
		if c.Visible && (c.Rect.W <= 0 || c.Rect.H <= 0) {
			t.Fatalf("%q: visible but unplaced %+v", c.Key, c.Rect)
		}
	}
	if control(t, cs, "tenant").Visible {
		t.Fatal("the tenant pop-up is shown with no tenant to choose")
	}
	if got := control(t, cs, "provider").Items; !same(got, providerLabels()...) {
		t.Fatalf("provider items %v", got)
	}
	_, _, cs = settingsControls(t, "oidc")
	if got := keys(cs); !same(got, "server_url", "provider", "oidc_issuer", "oidc_client_id") {
		t.Fatalf("oidc provider: native controls %v", got)
	}
}

func TestNativeTextReachesTheViewModelAndBack(t *testing.T) {
	v, vm, cs := settingsControls(t, "github")
	control(t, cs, "server_url").OnText("https://other.example.org")
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
}

// Choosing a provider in the pop-up switches the panel under it, and the
// pop-up follows the view model back.
func TestNativeProviderPopUpChoosesTheProvider(t *testing.T) {
	v, vm, cs := settingsControls(t, "github")
	control(t, cs, "provider").OnText(Providers[1].Label)
	if got := vm.ProviderIndex.Get(); got != 1 {
		t.Fatalf("provider index %d", got)
	}
	frame(t, v)
	cs = v.NativeControls()
	if got := keys(cs); !same(got, "server_url", "provider", "oidc_issuer", "oidc_client_id") {
		t.Fatalf("after choosing OIDC: %v", got)
	}
	vm.ProviderIndex.Set(0)
	if got := control(t, v.NativeControls(), "provider").Text; got != Providers[0].Label {
		t.Fatalf("pop-up shows %q", got)
	}
}

func TestClaimingStandsTheFallbackDown(t *testing.T) {
	v, _, cs := settingsControls(t, "github")
	frame(t, v)
	control(t, cs, "server_url").OnClaim(true)
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
