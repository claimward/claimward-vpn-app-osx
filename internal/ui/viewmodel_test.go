// SPDX-License-Identifier: BSD-3-Clause
//
// The ViewModel is an adapter over appvm (claimward-vpn-client pkg/appvm,
// tested there): these tests check what the adapter does -- every State is
// copied into the observables, every command reaches the model, and what the
// app adds (approval, preview, logos, exclusive servers) -- over a fake Core,
// with the results carried to the "UI thread" through a Queue.

package ui

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/claimward/claimward-vpn-client/pkg/appcore"
	"github.com/claimward/claimward-vpn-client/pkg/appvm"
	"github.com/claimward/claimward-vpn-client/pkg/exclusive"
	"github.com/claimward/claimward-vpn-client/pkg/helperclient"
	"github.com/claimward/claimward-vpn-client/pkg/hproto"

	"github.com/claimward/claimward-vpn-app-osx/internal/backend"
)

// shownBit says whether the detail panel lays out that bit.
func shownBit(vm *ViewModel, bit int) bool { return visible(vm.Shown.Get(), bit) }

func rows(vm *ViewModel) []string { return vm.Servers.Slice() }

func TestTheServerListShowsNameAndState(t *testing.T) {
	f := newCore(up(alpha()), in(beta()), func() appcore.ServerStatus { s := alpha(); s.URL, s.Name = urlC, "Gamma"; return s }())
	vm, _ := newVM(t, f)
	want := []string{"Alpha, Connected", "Beta, Signed in", "Gamma, Signed out"}
	if got := rows(vm); !reflect.DeepEqual(got, want) {
		t.Fatalf("rows %q, want %q", got, want)
	}
	if vm.ServerIndex.Get() != 0 || vm.ServerName.Get() != "Alpha" {
		t.Errorf("selected %d %q", vm.ServerIndex.Get(), vm.ServerName.Get())
	}
}

func TestSelectingAServerShowsItsDetail(t *testing.T) {
	f := twoServers()
	vm, q := newVM(t, f)
	if got := vm.Facts[factURL].Get(); got != urlA {
		t.Fatalf("url %q", got)
	}
	vm.ServerIndex.Set(1)
	settle(vm, q)
	if vm.ServerName.Get() != "Beta" || vm.Facts[factURL].Get() != urlB || vm.Facts[factAccount].Get() != "Not signed in" {
		t.Fatalf("detail %q %q %q", vm.ServerName.Get(), vm.Facts[factURL].Get(), vm.Facts[factAccount].Get())
	}
	// Syncing is not choosing: applying a State does not select again.
	vm.Refresh()
	settle(vm, q)
	if vm.ServerIndex.Get() != 1 {
		t.Fatalf("index %d after a refresh", vm.ServerIndex.Get())
	}
	// A row that is no server selects nothing.
	vm.ServerIndex.Set(7)
	settle(vm, q)
	if vm.ServerName.Get() != "Beta" {
		t.Fatalf("name %q after a row past the list", vm.ServerName.Get())
	}
}

func TestWithoutAServerThereIsNothingToActOn(t *testing.T) {
	vm, q := newVM(t, newCore())
	if vm.ServerName.Get() != "No server" || len(rows(vm)) != 0 {
		t.Errorf("name %q rows %q", vm.ServerName.Get(), rows(vm))
	}
	for i := 0; i < nFacts; i++ {
		if got := vm.Facts[i].Get(); got != "" && i != factStatus {
			t.Errorf("fact %s is %q with no server", factKeys[i], got)
		}
	}
	if vm.Shown.Get() != 1<<factStatus|1<<shownProvider || vm.SignInWith[0].CanExecute() {
		t.Errorf("shown %b: only the status and a sign-in button that does nothing", vm.Shown.Get())
	}
	if vm.Facts[factStatus].Get() != "Not configured" {
		t.Errorf("status %q", vm.Facts[factStatus].Get())
	}
	if vm.EditServer.CanExecute() || vm.RemoveServer.CanExecute() || !vm.AddServer.CanExecute() {
		t.Error("gear or − enabled with no server, or + not")
	}
	if vm.EditServerLabel.Get() != editServerNoChoice {
		t.Errorf("gear named %q", vm.EditServerLabel.Get())
	}
	run(vm, q, vm.EditServer.Execute)
	if vm.Page.Get() != PageStatus {
		t.Error("the gear opened the form with no server")
	}
}

func TestTheDetailPanelsFacts(t *testing.T) {
	now := time.Date(2026, 11, 27, 10, 0, 0, 0, time.UTC)
	f := signedIn()
	f.approved[urlA] = true
	f.leases[urlA] = now.Add(4*time.Minute + 30*time.Second)
	vm, q := newVM(t, f, WithClock(func() time.Time { return now }))
	want := map[int]string{
		factURL: urlA, factProvider: "GitHub", factAccount: "Signed in as ada@example.org",
		factApproval: "Approved on this Mac", factHelper: "Privileged helper: running",
		factTenant: "Tenant: the server's choice", factStatus: "Disconnected",
	}
	for i := 0; i < nFacts; i++ {
		if got := vm.Facts[i].Get(); got != want[i] {
			t.Errorf("fact %s = %q, want %q", factKeys[i], got, want[i])
		}
		// A fact shows when it has a value; the status always does.
		if shownBit(vm, i) != (want[i] != "") {
			t.Errorf("fact %s shown = %v", factKeys[i], shownBit(vm, i))
		}
	}
	if vm.StatusKind.Get() != StatusIdle || vm.TrayStatus.Get() != "Claimward: disconnected" {
		t.Errorf("kind %d tray %q", vm.StatusKind.Get(), vm.TrayStatus.Get())
	}
	if !vm.ConfirmRevoke.CanExecute() {
		t.Error("revoke is not on offer for an approved server")
	}

	// Connected: the address and the lease show, the lease relative to the
	// clock, which Tick moves on.
	f.edit(urlA, func(s *appcore.ServerStatus) {
		s.Connected, s.AssignedIP, s.Interface, s.Exclusive = true, "10.80.0.7/32", "Claimward", true
	})
	vm.Refresh()
	settle(vm, q)
	if got := vm.Facts[factAddress].Get(); got != "10.80.0.7/32 on Claimward" {
		t.Errorf("address %q", got)
	}
	if got := vm.Facts[factLease].Get(); got != "in 4 min" {
		t.Errorf("lease %q", got)
	}
	if got := vm.Facts[factExclusive].Get(); !strings.HasPrefix(got, "Yes") {
		t.Errorf("exclusive %q", got)
	}
	if vm.StatusKind.Get() != StatusUp {
		t.Errorf("kind %d", vm.StatusKind.Get())
	}
	now = now.Add(10 * time.Minute)
	vm.Tick()
	if got := vm.Facts[factLease].Get(); got != "expired" {
		t.Errorf("lease %q after the lease ended", got)
	}
	f.leases[urlA] = time.Time{}
	vm.Refresh()
	settle(vm, q)
	if vm.Facts[factLease].Get() != "" || shownBit(vm, factLease) {
		t.Error("a lease row shows with no lease")
	}
}

// The peer section shows what the live tunnel says, hides what is empty, and
// its relative time moves on with the clock.
func TestThePeerFactsShowTheTunnelAndHideTheEmpty(t *testing.T) {
	now := time.Date(2026, 11, 27, 10, 0, 0, 0, time.UTC)
	f := twoServers()
	vm, q := newVM(t, f, WithClock(func() time.Time { return now }))
	for _, i := range []int{factDNS, factMTU, factEndpoint, factAllowedIPs, factHandshake, factTransfer} {
		if vm.Facts[i].Get() != "" || shownBit(vm, i) {
			t.Errorf("%s shows while disconnected: %q", factKeys[i], vm.Facts[i].Get())
		}
	}
	shake := now.Add(-12 * time.Second)
	f.edit(urlA, func(s *appcore.ServerStatus) {
		s.Connected, s.AssignedIP, s.Interface = true, "10.80.0.7/32", "Claimward"
		s.Endpoint, s.AllowedIPs, s.DNS, s.MTU = "vpn.example.org:51820", []string{"10.0.0.0/8", "192.168.0.0/16"}, []string{"10.80.0.1"}, 1420
		s.RxBytes, s.TxBytes, s.LastHandshake = 3*1024*1024+512*1024, 900, &shake
	})
	vm.Refresh()
	settle(vm, q)
	for i, want := range map[int]string{
		factDNS: "10.80.0.1", factMTU: "1420", factEndpoint: "vpn.example.org:51820",
		factAllowedIPs: "10.0.0.0/8, 192.168.0.0/16", factHandshake: "12 s ago",
		factTransfer: "3.50 MiB received, 900 B sent",
	} {
		if got := vm.Facts[i].Get(); got != want {
			t.Errorf("%s = %q, want %q", factKeys[i], got, want)
		}
		if !shownBit(vm, i) {
			t.Errorf("%s is hidden", factKeys[i])
		}
	}
	now = now.Add(48 * time.Second)
	vm.Tick()
	if got := vm.Facts[factHandshake].Get(); got != "1 min 0 s ago" {
		t.Errorf("handshake %q a minute on", got)
	}
	// A peer that never completed a handshake has no such row.
	f.edit(urlA, func(s *appcore.ServerStatus) { s.LastHandshake = nil })
	vm.Refresh()
	settle(vm, q)
	if vm.Facts[factHandshake].Get() != "" || shownBit(vm, factHandshake) {
		t.Error("a handshake row shows with no handshake")
	}
}

func TestAgoAndSize(t *testing.T) {
	for d, want := range map[time.Duration]string{
		0: "just now", 12 * time.Second: "12 s ago", 125 * time.Second: "2 min 5 s ago",
		2*time.Hour + 3*time.Minute: "2 h 3 min ago",
	} {
		if got := ago(d); got != want {
			t.Errorf("ago(%v) = %q, want %q", d, got, want)
		}
	}
	for n, want := range map[uint64]string{
		0: "0 B", 1023: "1023 B", 1024: "1.00 KiB", 1536: "1.50 KiB",
		1 << 20: "1.00 MiB", 5 << 30: "5.00 GiB", 1 << 62: "4.00 EiB",
	} {
		if got := size(n); got != want {
			t.Errorf("size(%d) = %q, want %q", n, got, want)
		}
	}
}

// LogRows is Log through the wrap function: nothing is wider than the log.
func TestTheLogWrapsToItsRows(t *testing.T) {
	vm, _ := newVM(t, twoServers())
	vm.Log.Clear()
	vm.Log.Append("one", "two words")
	vm.rewrapLog()
	if got := vm.LogRows.Slice(); !reflect.DeepEqual(got, []string{"one", "two words"}) {
		t.Fatalf("unwrapped rows %q", got)
	}
	vm.SetLogWrap(func(s string) []string { return strings.Fields(s) })
	if got := vm.LogRows.Slice(); !reflect.DeepEqual(got, []string{"one", "two", "words"}) {
		t.Fatalf("wrapped rows %q", got)
	}
	touched := 0
	vm.LogRows.SubscribeChanged(func() { touched++ })
	vm.rewrapLog() // unchanged: the list is not touched again
	if touched != 0 {
		t.Errorf("an unchanged log rewrote its rows %d times", touched)
	}
	vm.SetLogWrap(nil)
	if got := vm.LogRows.Slice(); len(got) != 2 {
		t.Fatalf("rows %q after the wrap was removed", got)
	}
}

func TestRelativeTime(t *testing.T) {
	for d, want := range map[time.Duration]string{
		-time.Second:                       "expired",
		0:                                  "expired",
		42 * time.Second:                   "in 42 s",
		time.Minute:                        "in 1 min",
		59*time.Minute + 59*time.Second:    "in 59 min",
		time.Hour:                          "in 1 h 0 min",
		2*time.Hour + 5*time.Minute + 1000: "in 2 h 5 min",
	} {
		if got := relative(d); got != want {
			t.Errorf("relative(%v) = %q, want %q", d, got, want)
		}
	}
}

func TestAProblemIsTheStatusIcon(t *testing.T) {
	f := signedIn()
	f.failWith("Connect", errBoom)
	vm, q := newVM(t, f)
	run(vm, q, vm.Connect.Execute)
	if vm.Message.Get() != "Connection failed: boom" || vm.StatusKind.Get() != StatusProblem || vm.Busy.Get() != "" {
		t.Errorf("message %q kind %d busy %q", vm.Message.Get(), vm.StatusKind.Get(), vm.Busy.Get())
	}
}

func TestTheHelpersRefusalIsSaid(t *testing.T) {
	f := signedIn()
	f.failWith("Connect", &helperclient.NotApprovedError{Server: urlA, Message: "server not approved"})
	vm, q := newVM(t, f)
	run(vm, q, vm.Connect.Execute)
	if got := vm.Message.Get(); got != "Connection failed: helper: server not approved" {
		t.Errorf("message %q", got)
	}
	// The person who did not approve: the backend's wording is carried as it is.
	f.failWith("Connect", fmt.Errorf("%w: you did not approve a.example.org (cancelled)", backend.ErrNotApproved))
	run(vm, q, vm.Connect.Execute)
	if got := vm.Message.Get(); !strings.Contains(got, "not approved: you did not approve a.example.org") {
		t.Errorf("message %q", got)
	}
}

func TestOneSignInButtonPerProvider(t *testing.T) {
	f := newCore(alpha(), beta())
	vm, q := newVM(t, f)
	// One provider: a plain "Sign in"; the others have no title and no place.
	labels := func() []string {
		var out []string
		for _, l := range vm.ProviderLabels {
			out = append(out, l.Get())
		}
		return out
	}
	if got := labels(); !reflect.DeepEqual(got, []string{"Sign in", "", "", ""}) {
		t.Fatalf("labels %q", got)
	}
	if !shownBit(vm, shownProvider) || shownBit(vm, shownProvider+1) {
		t.Errorf("shown %b", vm.Shown.Get())
	}
	// Two providers: one button each.
	vm.ServerIndex.Set(1)
	settle(vm, q)
	if got := labels(); !reflect.DeepEqual(got, []string{"Sign in with GitHub", "Sign in with Campus", "", ""}) {
		t.Fatalf("labels %q", got)
	}
	if !shownBit(vm, shownProvider) || !shownBit(vm, shownProvider+1) || shownBit(vm, shownProvider+2) {
		t.Errorf("shown %b", vm.Shown.Get())
	}
	if vm.Facts[factProvider].Get() != "Not chosen yet" {
		t.Errorf("provider fact %q", vm.Facts[factProvider].Get())
	}
	// Signed in: the buttons are gone.
	vm.ServerIndex.Set(0)
	f.edit(urlA, func(s *appcore.ServerStatus) { s.LoggedIn = true })
	vm.Refresh()
	settle(vm, q)
	if shownBit(vm, shownProvider) {
		t.Error("a sign-in button on a signed-in server")
	}
}

func TestSigningInWithAProviderNotInForceChoosesItFirst(t *testing.T) {
	f := twoServers()
	vm, q := newVM(t, f)
	vm.ServerIndex.Set(1)
	settle(vm, q)
	if !vm.SignInWith[1].CanExecute() {
		t.Fatal("the second provider's button is off")
	}
	run(vm, q, vm.SignInWith[1].Execute)
	if c := f.last("SetProvider"); c.server != urlB || c.arg != "campus" {
		t.Fatalf("SetProvider %+v", c)
	}
	if c := f.last("Login"); c.server != urlB {
		t.Fatalf("Login %+v", c)
	}
	if vm.pending != -1 || rows(vm)[1] != "Beta, Signed in" {
		t.Errorf("pending %d rows %q", vm.pending, rows(vm))
	}
}

func TestSigningInWithTheProviderInForceSignsInAtOnce(t *testing.T) {
	f := newCore(alpha(), func() appcore.ServerStatus { b := beta(); b.Provider = "campus"; return b }())
	vm, q := newVM(t, f)
	vm.ServerIndex.Set(1)
	settle(vm, q)
	run(vm, q, vm.SignInWith[1].Execute)
	if f.called("SetProvider") != 0 || f.called("Login") != 1 {
		t.Errorf("SetProvider %d Login %d", f.called("SetProvider"), f.called("Login"))
	}
	// A server listing one provider needs none chosen.
	run(vm, q, vm.SignInWith[0].Execute) // Beta is signed in now: nothing runs
	f.waitFor(t, "Login", 1)
}

func TestAProviderThatCannotBeChosenCancelsTheSignIn(t *testing.T) {
	f := twoServers()
	f.failWith("SetProvider", errBoom)
	vm, q := newVM(t, f)
	vm.ServerIndex.Set(1)
	settle(vm, q)
	run(vm, q, vm.SignInWith[0].Execute)
	if f.called("Login") != 0 || vm.pending != -1 || !strings.Contains(vm.Message.Get(), "Choosing the identity provider failed") {
		t.Errorf("Login %d pending %d message %q", f.called("Login"), vm.pending, vm.Message.Get())
	}
}

func TestAPendingSignInWaitsForTheChoice(t *testing.T) {
	vm, _ := newVM(t, twoServers())
	two := []appvm.ProviderOption{{ID: "a", Label: "A"}, {ID: "b", Label: "B"}}
	vm.pending = 1
	vm.resolvePending(appvm.State{Busy: "Choosing…", ProviderOptions: two, ProviderIndex: 1})
	if vm.pending != 1 {
		t.Error("the sign-in ran while the choice was still being made")
	}
	vm.resolvePending(appvm.State{ProviderOptions: two[:1]}) // the list shrank under it
	if vm.pending != -1 {
		t.Error("a pending sign-in outlived its provider")
	}
}

// Nothing pending: nothing happens, even in a State that matches -1, over a
// server the model would let the person sign in to.
func TestNothingPendingSignsNoOneIn(t *testing.T) {
	f := signedOut()
	vm, q := newVM(t, f)
	vm.resolvePending(appvm.State{ProviderOptions: []appvm.ProviderOption{{ID: "a", Label: "A"}}, ProviderIndex: -1})
	settle(vm, q)
	if n := f.called("Login"); n != 0 {
		t.Errorf("a sign-in ran with nothing pending: %d Login call(s)", n)
	}
}

func TestTheDeviceCodeCard(t *testing.T) {
	f := signedOut()
	var opened []string
	vm, q := newVM(t, f, WithOpener(func(u string) error { opened = append(opened, u); return nil }))
	if vm.SignInPanel.Get() != PanelHidden {
		t.Fatal("the sign-in card shows before a sign-in")
	}
	f.hoist("Login")
	vm.SignInWith[0].Execute()
	f.waitFor(t, "Login", 1)
	f.edit(urlA, func(s *appcore.ServerStatus) {
		s.DeviceVerificationURI, s.DeviceUserCode = "https://github.com/login/device", "WDJB-MJHT"
	})
	vm.Refresh()
	until(t, q, func() bool { return vm.DeviceCode.Get() != "" })
	if vm.SignInPanel.Get() != PanelShown || vm.DeviceCode.Get() != "WDJB-MJHT" || vm.DeviceURI.Get() != "https://github.com/login/device" {
		t.Fatalf("card %d code %q uri %q", vm.SignInPanel.Get(), vm.DeviceCode.Get(), vm.DeviceURI.Get())
	}
	if vm.Busy.Get() == "" || !vm.CancelSignIn.CanExecute() || !vm.OpenSignInPage.CanExecute() {
		t.Error("a sign-in in progress offers no cancel or page")
	}
	vm.OpenSignInPage.Execute()
	if len(opened) != 1 || opened[0] != "https://github.com/login/device" {
		t.Errorf("opened %q", opened)
	}
	vm.CancelSignIn.Execute()
	settle(vm, q)
	if vm.Message.Get() != "Sign-in cancelled." {
		t.Errorf("message %q", vm.Message.Get())
	}
	f.edit(urlA, func(s *appcore.ServerStatus) { s.DeviceVerificationURI, s.DeviceUserCode = "", "" })
	vm.Refresh()
	settle(vm, q)
	if vm.SignInPanel.Get() != PanelHidden || vm.DeviceCode.Get() != "" {
		t.Error("the card outlives the sign-in")
	}
}

func TestChoosingATenant(t *testing.T) {
	f := signedIn()
	f.offered = []hproto.Tenant{{ID: "chem", Name: "Chemistry"}, {ID: "hpc", Name: "HPC"}}
	vm, q := newVM(t, f)
	if shownBit(vm, shownTenant) {
		t.Fatal("the tenant choice shows before the server asked for one")
	}
	run(vm, q, vm.Connect.Execute)
	if !strings.Contains(vm.Message.Get(), "several tenants") || !shownBit(vm, shownTenant) {
		t.Fatalf("message %q shown %b", vm.Message.Get(), vm.Shown.Get())
	}
	// While the choice is on offer the tenant is not also a fact.
	if shownBit(vm, factTenant) {
		t.Error("the tenant is both a fact and a choice")
	}
	if got := vm.TenantNames.Slice(); !reflect.DeepEqual(got, []string{"Choose a tenant…", "Chemistry (chem)", "HPC (hpc)"}) {
		t.Fatalf("tenant rows %q", got)
	}
	// The pop-up reports the title the person picked; a title that is no row is
	// not a choice.
	vm.TenantName.Set("Nothing of the kind")
	q.Drain()
	if f.called("SetTenant") != 0 {
		t.Fatal("an unknown title chose a tenant")
	}
	vm.TenantName.Set("HPC (hpc)")
	q.Drain()
	if c := f.last("SetTenant"); c.server != urlA || c.arg != "hpc" || vm.TenantIndex.Get() != 2 {
		t.Fatalf("SetTenant %+v index %d", c, vm.TenantIndex.Get())
	}
	// Saying the title of the row already chosen is not choosing again.
	vm.TenantName.Set("")
	vm.TenantName.Set("HPC (hpc)")
	q.Drain()
	if f.called("SetTenant") != 1 {
		t.Fatalf("the same row chosen %d times", f.called("SetTenant"))
	}
	run(vm, q, vm.Connect.Execute)
	if vm.Facts[factTenant].Get() != "Tenant: HPC (hpc)" || shownBit(vm, shownTenant) {
		t.Errorf("tenant %q shown %b", vm.Facts[factTenant].Get(), vm.Shown.Get())
	}
}

func TestRefreshingTheTenants(t *testing.T) {
	f := signedIn()
	f.offered = []hproto.Tenant{{ID: "chem"}, {ID: "hpc"}}
	vm, q := newVM(t, f)
	run(vm, q, vm.ChooseTenant.Execute)
	if f.last("Tenants").server != urlA || !shownBit(vm, shownTenant) {
		t.Errorf("Tenants %+v shown %b", f.last("Tenants"), vm.Shown.Get())
	}
}

// A row the model reports that the list does not hold shows as no title.
func TestAnIndexOutOfRangeShowsNoTitle(t *testing.T) {
	vm, _ := newVM(t, signedIn())
	st := vm.m.State()
	st.TenantIndex, st.ProviderIndex = 9, -1
	vm.apply(st)
	if vm.TenantName.Get() != "" || vm.ProviderName.Get() != "" {
		t.Fatalf("titles %q %q for a row past the list", vm.TenantName.Get(), vm.ProviderName.Get())
	}
}

// The model's options change without their number changing: the rows follow.
func TestARenamedTenantRowIsShown(t *testing.T) {
	vm, _ := newVM(t, signedIn())
	st := vm.m.State()
	st.TenantOptions = []appvm.TenantOption{{ID: "a", Label: "A"}, {ID: "b", Label: "B"}}
	vm.apply(st)
	st.TenantOptions = []appvm.TenantOption{{ID: "a", Label: "A"}, {ID: "c", Label: "C"}}
	vm.apply(st)
	if got := vm.TenantNames.Slice(); got[1] != "C" {
		t.Fatalf("rows %v", got)
	}
}

// Two rows can carry the same title; saying the title of the row already
// chosen leaves the choice on that row.
func TestATitleOfTheChosenRowKeepsTheRow(t *testing.T) {
	vm, _ := newVM(t, signedIn())
	st := vm.m.State()
	st.TenantOptions = []appvm.TenantOption{{ID: "a", Label: "X"}, {ID: "b", Label: "X"}}
	st.TenantIndex = 1
	vm.apply(st)
	vm.TenantName.Set("")
	vm.TenantName.Set("X")
	if got := vm.TenantIndex.Get(); got != 1 {
		t.Fatalf("index %d, want the chosen row 1", got)
	}
}

// Connect waits on the servers that exclude each other.
func TestConnectIsOffWhileExclusiveServersAreInTheWay(t *testing.T) {
	a := up(alpha())
	a.Exclusive = true
	f := newCore(a, in(beta()))
	vm, q := newVM(t, f)
	vm.ServerIndex.Set(1)
	settle(vm, q)
	if vm.Connect.CanExecute() {
		t.Error("Connect is on while an exclusive server is connected")
	}
	if note := vm.ConnectNote.Get(); !strings.Contains(note, "Alpha") || !shownBit(vm, shownNote) {
		t.Errorf("note %q shown %b", note, vm.Shown.Get())
	}
	vm.Connect.Execute()
	settle(vm, q)
	if f.called("Connect") != 0 {
		t.Error("a disabled Connect connected")
	}

	// The server itself claims it: the same, naming it.
	f2 := newCore(up(alpha()), in(func() appcore.ServerStatus { b := beta(); b.Exclusive = true; return b }()))
	vm2, q2 := newVM(t, f2)
	vm2.ServerIndex.Set(1)
	settle(vm2, q2)
	if note := vm2.ConnectNote.Get(); !strings.Contains(note, "Beta") || vm2.Connect.CanExecute() {
		t.Errorf("note %q", note)
	}

	// The connected server itself needs no note; nor do servers that share.
	vm2.ServerIndex.Set(0)
	settle(vm2, q2)
	if vm2.ConnectNote.Get() != "" || shownBit(vm2, shownNote) {
		t.Errorf("note %q on the connected server", vm2.ConnectNote.Get())
	}
	f3 := newCore(up(alpha()), in(beta()))
	vm3, q3 := newVM(t, f3)
	vm3.ServerIndex.Set(1)
	settle(vm3, q3)
	if vm3.ConnectNote.Get() != "" || !vm3.Connect.CanExecute() {
		t.Fatalf("note %q, Connect %v", vm3.ConnectNote.Get(), vm3.Connect.CanExecute())
	}
	run(vm3, q3, vm3.Connect.Execute)
	if f3.last("Connect").server != urlB {
		t.Errorf("Connect %+v", f3.last("Connect"))
	}
}

func TestExclusiveNote(t *testing.T) {
	sel := appvm.ServerRow{URL: urlA, Name: "A", Selected: true}
	other := appvm.ServerRow{URL: urlB, Name: "B", Connected: true}
	if got := exclusiveNote(nil); got != "" {
		t.Errorf("no rows: %q", got)
	}
	if got := exclusiveNote([]appvm.ServerRow{other}); got != "" {
		t.Errorf("no selection: %q", got)
	}
	if got := exclusiveNote([]appvm.ServerRow{sel, other}); got != "" {
		t.Errorf("sharing: %q", got)
	}
	other.Exclusive = true
	got := exclusiveNote([]appvm.ServerRow{sel, other})
	want := exclusive.Check(exclusive.Member{URL: urlA, Name: "A"}, []exclusive.Member{{URL: urlB, Name: "B", Exclusive: true}}).Error()
	if got != want {
		t.Errorf("note %q, want %q", got, want)
	}
}

func TestAddingAServer(t *testing.T) {
	f := signedIn()
	f.previews[urlC] = backend.Preview{Name: "Gamma", Providers: []string{"GitHub", "Campus"}, Exclusive: true, Logo: []byte("<svg id='c'/>")}
	vm, q := newVM(t, f)
	run(vm, q, vm.AddServer.Execute)
	if vm.Page.Get() != PageSettings || vm.FormCard.Get() != FormURL || vm.ServerURL.Get() != "" {
		t.Fatalf("page %d card %d url %q", vm.Page.Get(), vm.FormCard.Get(), vm.ServerURL.Get())
	}
	// Nothing typed: Next says what to type.
	run(vm, q, vm.Next.Execute)
	if !strings.Contains(vm.FormMsg.Get(), "Type the server's address") || f.called("Preview") != 0 {
		t.Fatalf("form message %q", vm.FormMsg.Get())
	}
	vm.ServerURL.Set("  " + urlC + " ")
	vm.Next.Execute()
	if vm.FormMsg.Get() == "" {
		t.Error("no sign that the description is being read")
	}
	settle(vm, q)
	if vm.FormCard.Get() != FormPreview || vm.FormMsg.Get() != "" || vm.PreviewName.Get() != "Gamma" {
		t.Fatalf("card %d msg %q name %q", vm.FormCard.Get(), vm.FormMsg.Get(), vm.PreviewName.Get())
	}
	if got := vm.PreviewDetail.Get(); got != "Sign-in with GitHub, Campus. Exclusive: it requires being the only connection." {
		t.Errorf("detail %q", got)
	}
	if vm.PreviewLogoDoc() != "<svg id='c'/>" || vm.LogoTick.Get() == 0 {
		t.Errorf("preview logo %q tick %d", vm.PreviewLogoDoc(), vm.LogoTick.Get())
	}
	if f.last("Preview").server != urlC {
		t.Errorf("previewed %q", f.last("Preview").server)
	}

	// Back to the address, and forward again.
	run(vm, q, vm.Back.Execute)
	if vm.FormCard.Get() != FormURL {
		t.Fatalf("card %d after Back", vm.FormCard.Get())
	}
	run(vm, q, vm.Next.Execute)
	run(vm, q, vm.ConfirmAdd.Execute)
	if f.last("AddServer").server != urlC {
		t.Fatalf("AddServer %+v", f.last("AddServer"))
	}
	if vm.Page.Get() != PageStatus || vm.ServerName.Get() != "Gamma" || !reflect.DeepEqual(rows(vm), []string{"Alpha, Signed in", "Gamma, Signed out"}) {
		t.Errorf("page %d name %q rows %q", vm.Page.Get(), vm.ServerName.Get(), rows(vm))
	}

	// + again starts from nothing.
	run(vm, q, vm.AddServer.Execute)
	if vm.ServerURL.Get() != "" || vm.PreviewName.Get() != "" || vm.PreviewDetail.Get() != "" || vm.PreviewLogoDoc() != "" {
		t.Error("the previous preview is still there")
	}
	run(vm, q, vm.CloseSettings.Execute)
	if vm.Page.Get() != PageStatus {
		t.Error("Cancel did not leave the form")
	}
}

func TestAServerThatDoesNotAnswerIsSaid(t *testing.T) {
	f := signedIn()
	f.failWith("Preview", errBoom)
	vm, q := newVM(t, f)
	run(vm, q, vm.AddServer.Execute)
	vm.ServerURL.Set(urlC)
	run(vm, q, vm.Next.Execute)
	if got := vm.FormMsg.Get(); got != "c.example.org did not answer as a Claimward server: boom" {
		t.Errorf("form message %q", got)
	}
	if vm.FormCard.Get() != FormURL {
		t.Errorf("card %d", vm.FormCard.Get())
	}
}

func TestAServerThatCannotBeAddedKeepsTheFormOpen(t *testing.T) {
	f := signedIn()
	f.failWith("AddServer", errBoom)
	vm, q := newVM(t, f)
	run(vm, q, vm.AddServer.Execute)
	// Added without being previewed: the address is not one.
	run(vm, q, vm.ConfirmAdd.Execute)
	if !strings.Contains(vm.FormMsg.Get(), "must be an https URL") {
		t.Errorf("form message %q", vm.FormMsg.Get())
	}
	vm.ServerURL.Set(urlC)
	run(vm, q, vm.Next.Execute)
	run(vm, q, vm.ConfirmAdd.Execute)
	if vm.Page.Get() != PageSettings || vm.FormMsg.Get() != "Adding the server failed: boom" || vm.Message.Get() != "Adding the server failed: boom" {
		t.Errorf("page %d form %q message %q", vm.Page.Get(), vm.FormMsg.Get(), vm.Message.Get())
	}
}

func TestPreviewDetail(t *testing.T) {
	for _, c := range []struct {
		p    backend.Preview
		want string
	}{
		{backend.Preview{}, "No sign-in provider listed"},
		{backend.Preview{Providers: []string{"GitHub"}}, "Sign-in with GitHub"},
		{backend.Preview{Exclusive: true}, "No sign-in provider listed. Exclusive: it requires being the only connection."},
	} {
		if got := previewDetail(c.p); got != c.want {
			t.Errorf("%+v: %q, want %q", c.p, got, c.want)
		}
	}
}

func TestTheServerWindowHasAWindowOfItsOwn(t *testing.T) {
	opened, closed := 0, 0
	ok := true
	f := signedIn()
	f.previews[urlC] = backend.Preview{Name: "Gamma"}
	vm, q := newVM(t, f, WithSettingsWindow(func() (func(), bool) {
		opened++
		return func() { closed++ }, ok
	}))
	run(vm, q, vm.AddServer.Execute)
	if opened != 1 || !vm.SettingsInWindow.Get() || vm.Page.Get() != PageSettings || vm.PageShown.Get() != PageStatus {
		t.Fatalf("opened %d in window %v page %d shown %d", opened, vm.SettingsInWindow.Get(), vm.Page.Get(), vm.PageShown.Get())
	}
	vm.ServerURL.Set(urlC)
	run(vm, q, vm.Next.Execute)
	run(vm, q, vm.ConfirmAdd.Execute)
	if closed != 1 || vm.SettingsInWindow.Get() || vm.PageShown.Get() != PageStatus {
		t.Errorf("closed %d in window %v", closed, vm.SettingsInWindow.Get())
	}

	// The person closes the window: the form is cancelled.
	run(vm, q, vm.AddServer.Execute)
	vm.SettingsWindowClosed()
	settle(vm, q)
	if vm.Page.Get() != PageStatus || vm.SettingsInWindow.Get() || closed != 1 {
		t.Errorf("page %d in window %v closed %d", vm.Page.Get(), vm.SettingsInWindow.Get(), closed)
	}
	vm.SettingsWindowClosed() // no window: nothing to do
	// A window closed after the form already left its page is not a cancel.
	vm.closeWindow = func() {}
	vm.SettingsWindowClosed()
	if vm.closeWindow != nil {
		t.Error("the window is still held")
	}

	// A platform with one window: the form stays on the main window's page.
	ok = false
	run(vm, q, vm.AddServer.Execute)
	if vm.SettingsInWindow.Get() || vm.PageShown.Get() != PageSettings {
		t.Errorf("in window %v shown %d", vm.SettingsInWindow.Get(), vm.PageShown.Get())
	}
	// The form is open already: opening it again does nothing.
	vm.openAdd()
	if opened != 3 {
		t.Errorf("opened %d", opened)
	}
}

func TestEditingAServer(t *testing.T) {
	f := twoServers()
	vm, q := newVM(t, f)
	if vm.EditServerLabel.Get() != editServerName || !vm.EditServer.CanExecute() {
		t.Fatalf("gear %q on %v", vm.EditServerLabel.Get(), vm.EditServer.CanExecute())
	}
	vm.ServerIndex.Set(1)
	settle(vm, q)
	run(vm, q, vm.EditServer.Execute)
	if vm.Page.Get() != PageSettings || vm.FormCard.Get() != FormEdit || vm.ServerURL.Get() != urlB {
		t.Fatalf("page %d card %d url %q", vm.Page.Get(), vm.FormCard.Get(), vm.ServerURL.Get())
	}
	if got := vm.Providers.Slice(); !reflect.DeepEqual(got, []string{"GitHub", "Campus"}) || vm.ProviderName.Get() != "GitHub" {
		t.Fatalf("providers %q chosen %q", got, vm.ProviderName.Get())
	}
	vm.ProviderName.Set("Campus")
	settle(vm, q)
	if c := f.last("SetProvider"); c.server != urlB || c.arg != "campus" || vm.ProviderName.Get() != "Campus" || vm.ProviderIndex.Get() != 1 {
		t.Fatalf("SetProvider %+v chosen %q", c, vm.ProviderName.Get())
	}
	vm.ProviderName.Set("Nobody")
	if vm.ProviderIndex.Get() != 1 {
		t.Error("a title that is no provider moved the choice")
	}
	run(vm, q, vm.CloseSettings.Execute)
	if vm.Page.Get() != PageStatus {
		t.Error("Done did not leave the form")
	}
}

func TestRemovingAServer(t *testing.T) {
	f := twoServers()
	vm, q := newVM(t, f)
	if vm.RemovePanel.Get() != PanelHidden {
		t.Fatal("the removal card shows by itself")
	}
	run(vm, q, vm.RemoveServer.Execute)
	if vm.RemovePanel.Get() != PanelShown || vm.RemoveText.Get() != "Remove Alpha?" {
		t.Fatalf("card %d text %q", vm.RemovePanel.Get(), vm.RemoveText.Get())
	}
	// Not approved by the helper: nothing to revoke.
	if vm.ConfirmRevoke.CanExecute() {
		t.Error("revoke is on offer for a server the helper does not know")
	}
	vm.ConfirmRevoke.Execute()
	if f.called("Revoke") != 0 {
		t.Error("revoked an approval that does not exist")
	}
	run(vm, q, vm.CancelRemove.Execute)
	if vm.RemovePanel.Get() != PanelHidden || f.called("RemoveServer") != 0 {
		t.Fatal("Cancel removed the server")
	}
	run(vm, q, vm.RemoveServer.Execute)
	run(vm, q, vm.ConfirmRemove.Execute)
	if vm.RemovePanel.Get() != PanelHidden || f.last("RemoveServer").server != urlA || !reflect.DeepEqual(rows(vm), []string{"Beta, Signed out"}) {
		t.Errorf("card %d removed %+v rows %q", vm.RemovePanel.Get(), f.last("RemoveServer"), rows(vm))
	}
	if vm.ServerName.Get() != "Beta" {
		t.Errorf("name %q", vm.ServerName.Get())
	}
}

func TestRemovingAndRevokingAnApproval(t *testing.T) {
	f := signedIn()
	f.approved[urlA] = true
	vm, q := newVM(t, f)
	run(vm, q, vm.RemoveServer.Execute)
	if !vm.ConfirmRevoke.CanExecute() {
		t.Fatal("revoke is not on offer for an approved server")
	}
	// The helper refuses: the server stays, and the person is told why.
	f.failWith("Revoke", errBoom)
	run(vm, q, vm.ConfirmRevoke.Execute)
	if got := vm.Message.Get(); got != "Revoking the approval of a.example.org failed: boom" {
		t.Fatalf("message %q", got)
	}
	if f.called("RemoveServer") != 0 || len(rows(vm)) != 1 || vm.RemovePanel.Get() != PanelHidden {
		t.Error("the server was removed despite the refusal")
	}
	// The notice outlives a poll, and goes at the person's next action.
	vm.Refresh()
	settle(vm, q)
	if vm.Message.Get() == "" {
		t.Error("the notice did not outlive a poll")
	}
	run(vm, q, vm.EditServer.Execute)
	if vm.Message.Get() != "" {
		t.Errorf("message %q after the next action", vm.Message.Get())
	}
	run(vm, q, vm.CloseSettings.Execute)

	f.failWith("Revoke", nil)
	run(vm, q, vm.RemoveServer.Execute)
	run(vm, q, vm.ConfirmRevoke.Execute)
	if f.last("Revoke").server != urlA || f.called("RemoveServer") != 1 || len(rows(vm)) != 0 {
		t.Errorf("revoke %+v removed %d rows %q", f.last("Revoke"), f.called("RemoveServer"), rows(vm))
	}
	if vm.ServerName.Get() != "No server" {
		t.Errorf("name %q", vm.ServerName.Get())
	}
}

func TestLogosAreFetchedOncePerServerAndName(t *testing.T) {
	f := twoServers()
	f.logos[urlA] = []byte("<svg id='a'/>")
	vm, q := newVM(t, f)
	if vm.LogoDoc() != "<svg id='a'/>" || vm.RowLogo(0) != "<svg id='a'/>" || vm.RowLogo(1) != "" {
		t.Fatalf("logos %q %q %q", vm.LogoDoc(), vm.RowLogo(0), vm.RowLogo(1))
	}
	if vm.RowLogo(-1) != "" || vm.RowLogo(9) != "" {
		t.Error("a logo for a row that does not exist")
	}
	if f.called("Logo") != 2 || vm.LogoTick.Get() != 2 {
		t.Fatalf("Logo asked %d times, tick %d", f.called("Logo"), vm.LogoTick.Get())
	}
	vm.Refresh()
	vm.Refresh()
	settle(vm, q)
	if f.called("Logo") != 2 {
		t.Errorf("Logo asked %d times for servers already held", f.called("Logo"))
	}

	// The server's name arrives later (its document was read): ask again.
	f.logos[urlA] = []byte("<svg id='a2'/>")
	f.edit(urlA, func(s *appcore.ServerStatus) { s.Name = "Alpha Lab" })
	vm.Refresh()
	settle(vm, q)
	if vm.LogoDoc() != "<svg id='a2'/>" || f.called("Logo") != 3 {
		t.Errorf("logo %q after the rename, asked %d times", vm.LogoDoc(), f.called("Logo"))
	}

	// A server removed takes its logo with it.
	f.set(func(st *appcore.Status) { st.Servers = st.Servers[1:] })
	vm.Refresh()
	settle(vm, q)
	if _, held := vm.logos[urlA]; held || len(vm.fetched) != 1 || vm.RowLogo(0) != "" {
		t.Errorf("logos %v fetched %v", vm.logos, vm.fetched)
	}
}

// An answer that arrives for a name or a server that is not current any more
// is dropped.
func TestAStaleLogoIsDropped(t *testing.T) {
	f := signedIn()
	f.logos[urlA] = []byte("<svg id='old'/>")
	vm, q := newVM(t, f)
	tick := vm.LogoTick.Get()
	rename := func(name string) {
		f.edit(urlA, func(s *appcore.ServerStatus) { s.Name = name })
		vm.Refresh()
	}
	f.logos[urlA] = []byte("<svg id='x'/>")
	rename("X")
	f.logos[urlA] = []byte("<svg id='y'/>")
	rename("Y")
	// Both States wait in the queue: the answer to the first finds the second.
	settle(vm, q)
	if vm.LogoDoc() != "<svg id='y'/>" || vm.LogoTick.Get() != tick+1 {
		t.Errorf("logo %q tick %d, want y and %d", vm.LogoDoc(), vm.LogoTick.Get(), tick+1)
	}
	// The server goes before its answer comes.
	f.set(func(st *appcore.Status) { st.Servers[0].Name = "Z" })
	vm.Refresh()
	q.Drain()
	f.set(func(st *appcore.Status) { st.Servers = nil })
	vm.Refresh()
	settle(vm, q)
	if vm.LogoDoc() != "" || len(vm.logos) != 0 {
		t.Errorf("logo %q for a server that is gone", vm.LogoDoc())
	}
}

func TestTheConnectionLog(t *testing.T) {
	f := signedIn()
	f.st.Log = []string{"a", "b"}
	vm, q := newVM(t, f)
	step := func(log ...string) []string {
		f.set(func(st *appcore.Status) { st.Log = log })
		vm.Refresh()
		settle(vm, q)
		return vm.Log.Slice()
	}
	if got := vm.Log.Slice(); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("log %q", got)
	}
	if got := step("a", "b", "c"); !reflect.DeepEqual(got, []string{"a", "b", "c"}) {
		t.Errorf("appended: %q", got)
	}
	// The log is a ring: its oldest lines go, which is not a continuation.
	if got := step("b", "c", "d"); !reflect.DeepEqual(got, []string{"b", "c", "d"}) {
		t.Errorf("ring: %q", got)
	}
	if got := step(); len(got) != 0 {
		t.Errorf("emptied: %q", got)
	}
}

func TestTheModelsMessagesAreShown(t *testing.T) {
	vm, q := newVM(t, signedIn())
	// The settings error shows in the form and as the message.
	run(vm, q, vm.AddServer.Execute)
	run(vm, q, vm.ConfirmAdd.Execute)
	if vm.Message.Get() == "" || vm.Message.Get() != vm.FormMsg.Get() {
		t.Errorf("message %q form %q", vm.Message.Get(), vm.FormMsg.Get())
	}
	if vm.TrayStatus.Get() != "Claimward: disconnected" {
		t.Errorf("tray %q", vm.TrayStatus.Get())
	}
}

func TestOptions(t *testing.T) {
	f := signedIn()
	vm, _ := newVM(t, f, WithConfigPath("/etc/claimward.json"), WithInterval(2*time.Millisecond, time.Millisecond))
	if got := vm.Model().State().ConfigPath; got != "/etc/claimward.json" {
		t.Errorf("config path %q", got)
	}
	before := f.called("Status")
	vm.Start()
	f.waitFor(t, "Status", before+3)
	vm.Stop()
	vm.Wait()
}

func TestApplyShowsAStatus(t *testing.T) {
	vm := NewViewModel(newCore())
	vm.Apply(appcore.Status{ConfigOK: true, HelperInstalled: true, Servers: []appcore.ServerStatus{up(alpha())}})
	vm.Wait()
	if vm.Facts[factStatus].Get() != "Connected" || vm.TrayStatus.Get() != "Claimward: connected (10.80.0.7/32)" {
		t.Errorf("status %q tray %q", vm.Facts[factStatus].Get(), vm.TrayStatus.Get())
	}
}

func TestRowTextAndSplit(t *testing.T) {
	for _, c := range []struct {
		r           appvm.ServerRow
		name, state string
	}{
		{appvm.ServerRow{Name: "A"}, "A", "Signed out"},
		{appvm.ServerRow{Name: "A", LoggedIn: true}, "A", "Signed in"},
		{appvm.ServerRow{Name: "A, B", LoggedIn: true, Connected: true}, "A, B", "Connected"},
	} {
		name, state := splitRow(rowText(c.r))
		if name != c.name || state != c.state {
			t.Errorf("%+v: %q, %q", c.r, name, state)
		}
	}
	if name, state := splitRow("no state"); name != "no state" || state != "" {
		t.Errorf("%q %q", name, state)
	}
}

func TestProviderValue(t *testing.T) {
	st := appvm.State{ProviderOptions: []appvm.ProviderOption{{Label: "GitHub"}}}
	if got := providerValue(st); got != "GitHub" {
		t.Errorf("%q", got)
	}
	st.NeedsProvider = true
	if got := providerValue(st); got != "Not chosen yet" {
		t.Errorf("%q", got)
	}
	st.NeedsProvider, st.ProviderIndex = false, -1
	if got := providerValue(st); got != "Not chosen yet" {
		t.Errorf("%q", got)
	}
}
