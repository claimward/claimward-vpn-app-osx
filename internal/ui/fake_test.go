// SPDX-License-Identifier: BSD-3-Clause
//
// The fake Core every test of this package runs over: several servers, each
// with its own state, the calls recorded, errors injected by method name, and
// Login and Connect that can be held until the test lets them go.

package ui

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/claimward/claimward-vpn-client/pkg/appcore"
	"github.com/claimward/claimward-vpn-client/pkg/hproto"
	"github.com/claimward/claimward-vpn-client/pkg/protocol"

	"github.com/claimward/claimward-vpn-app-osx/internal/backend"
)

const (
	urlA = "https://a.example.org"
	urlB = "https://b.example.org"
	urlC = "https://c.example.org"
)

var (
	gitHub = protocol.Provider{ID: "gh", Name: "GitHub"}
	campus = protocol.Provider{ID: "campus", Name: "Campus"}
)

// call is one call on the fake: the method, and the server it was about.
type call struct{ name, server, arg string }

// fakeCore behaves as backend.Backend does as far as the ViewModel can see.
type fakeCore struct {
	mu sync.Mutex
	st appcore.Status

	// offered are the tenants every server lists.
	offered []hproto.Tenant
	// errs make the method of that name fail; waits hold Login and Connect
	// until the channel is closed or the call's context ends.
	errs  map[string]error
	waits map[string]chan struct{}

	approved map[string]bool
	leases   map[string]time.Time
	logos    map[string][]byte
	previews map[string]backend.Preview

	calls []call
}

func (f *fakeCore) record(name, server, arg string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, call{name, server, arg})
}

// hold waits for the method's channel, when the test set one.
func (f *fakeCore) hold(ctx context.Context, name string) error {
	f.mu.Lock()
	ch := f.waits[name]
	f.mu.Unlock()
	if ch == nil {
		return nil
	}
	select {
	case <-ch:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (f *fakeCore) fail(name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.errs[name]
}

// server is the status of url; the caller holds f.mu.
func (f *fakeCore) server(url string) *appcore.ServerStatus {
	for i := range f.st.Servers {
		if f.st.Servers[i].URL == url {
			return &f.st.Servers[i]
		}
	}
	return &appcore.ServerStatus{}
}

// Status hands out a copy: the model edits the servers it holds.
func (f *fakeCore) Status() appcore.Status {
	f.record("Status", "", "")
	f.mu.Lock()
	defer f.mu.Unlock()
	st := f.st
	st.Servers = append([]appcore.ServerStatus{}, f.st.Servers...)
	st.Log = append([]string(nil), f.st.Log...)
	return st
}

func (f *fakeCore) AddServer(_ context.Context, raw string) (appcore.ServerEntry, error) {
	f.record("AddServer", raw, "")
	if err := f.fail("AddServer"); err != nil {
		return appcore.ServerEntry{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	s := appcore.ServerStatus{URL: raw, Name: f.previews[raw].Name}
	f.st.Servers = append(f.st.Servers, s)
	return appcore.ServerEntry{URL: raw}, nil
}

func (f *fakeCore) RemoveServer(_ context.Context, server string) error {
	f.record("RemoveServer", server, "")
	if err := f.fail("RemoveServer"); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	var keep []appcore.ServerStatus
	for _, s := range f.st.Servers {
		if s.URL != server {
			keep = append(keep, s)
		}
	}
	f.st.Servers = keep
	return nil
}

func (f *fakeCore) SetProvider(_ context.Context, server, id string) error {
	f.record("SetProvider", server, id)
	if err := f.fail("SetProvider"); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.server(server).Provider = id
	return nil
}

// Login refuses a server that lists several providers when none is chosen.
func (f *fakeCore) Login(ctx context.Context, server, providerID string) error {
	f.record("Login", server, providerID)
	if err := f.hold(ctx, "Login"); err != nil {
		return err
	}
	if err := f.fail("Login"); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	s := f.server(server)
	if len(s.Providers) > 1 && s.Provider == "" {
		return appcore.ErrProviderRequired
	}
	s.LoggedIn, s.Email = true, "ada@example.org"
	s.DeviceVerificationURI, s.DeviceUserCode = "", ""
	return nil
}

func (f *fakeCore) Tenants(_ context.Context, server string) ([]hproto.Tenant, error) {
	f.record("Tenants", server, "")
	if err := f.fail("Tenants"); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.server(server).Tenants = append([]hproto.Tenant(nil), f.offered...)
	return f.offered, nil
}

func (f *fakeCore) SetTenant(server, id string) error {
	f.record("SetTenant", server, id)
	if err := f.fail("SetTenant"); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	s := f.server(server)
	s.Tenant, s.TenantRequired = id, false
	return nil
}

// Connect refuses a person in several tenants who chose none, as the server and
// appcore do, offering the tenants.
func (f *fakeCore) Connect(ctx context.Context, server string) error {
	f.record("Connect", server, "")
	if err := f.hold(ctx, "Connect"); err != nil {
		return err
	}
	if err := f.fail("Connect"); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	s := f.server(server)
	if len(f.offered) > 1 && s.Tenant == "" {
		s.Tenants, s.TenantRequired = append([]hproto.Tenant(nil), f.offered...), true
		return appcore.ErrTenantRequired
	}
	s.Connected, s.AssignedIP, s.Interface = true, "10.80.0.7/32", "Claimward"
	return nil
}

func (f *fakeCore) Disconnect(_ context.Context, server string) error {
	f.record("Disconnect", server, "")
	f.mu.Lock()
	defer f.mu.Unlock()
	s := f.server(server)
	s.Connected, s.AssignedIP, s.Interface = false, "", ""
	return f.errs["Disconnect"]
}

func (f *fakeCore) Logout(_ context.Context, server string) error {
	f.record("Logout", server, "")
	f.mu.Lock()
	defer f.mu.Unlock()
	s := f.server(server)
	s.LoggedIn, s.Email, s.Connected, s.Tenant = false, "", false, ""
	return f.errs["Logout"]
}

func (f *fakeCore) Logo(_ context.Context, server string) []byte {
	f.record("Logo", server, "")
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.logos[server]
}

func (f *fakeCore) Preview(_ context.Context, raw string) (backend.Preview, error) {
	f.record("Preview", raw, "")
	if err := f.fail("Preview"); err != nil {
		return backend.Preview{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	p := f.previews[raw]
	p.URL = raw
	return p, nil
}

func (f *fakeCore) Approved(server string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.approved[server]
}

func (f *fakeCore) LeaseExpires(server string) time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.leases[server]
}

func (f *fakeCore) Revoke(_ context.Context, server string) error {
	f.record("Revoke", server, "")
	if err := f.fail("Revoke"); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.approved, server)
	return nil
}

// set edits the status the fake reports.
func (f *fakeCore) set(fn func(st *appcore.Status)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn(&f.st)
}

// edit edits the server at url.
func (f *fakeCore) edit(url string, fn func(s *appcore.ServerStatus)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn(f.server(url))
}

func (f *fakeCore) failWith(name string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.errs[name] = err
}

// hoist makes Login or Connect wait; the returned function lets them go.
func (f *fakeCore) hoist(name string) (release func()) {
	ch := make(chan struct{})
	f.mu.Lock()
	f.waits[name] = ch
	f.mu.Unlock()
	var once sync.Once
	return func() { once.Do(func() { close(ch) }) }
}

func (f *fakeCore) called(name string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		if c.name == name {
			n++
		}
	}
	return n
}

// last is the latest call of that name.
func (f *fakeCore) last(name string) call {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := len(f.calls) - 1; i >= 0; i-- {
		if f.calls[i].name == name {
			return f.calls[i]
		}
	}
	return call{}
}

// waitFor blocks until the method has been called n times.
func (f *fakeCore) waitFor(t *testing.T, name string, n int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for f.called(name) < n {
		if time.Now().After(deadline) {
			t.Fatalf("%s was not called %d times", name, n)
		}
		time.Sleep(time.Millisecond)
	}
}

func newCore(servers ...appcore.ServerStatus) *fakeCore {
	return &fakeCore{
		st:       appcore.Status{ConfigOK: true, HelperInstalled: true, Servers: servers},
		errs:     map[string]error{},
		waits:    map[string]chan struct{}{},
		approved: map[string]bool{},
		leases:   map[string]time.Time{},
		logos:    map[string][]byte{},
		previews: map[string]backend.Preview{},
	}
}

// alpha is a server listing one provider, chosen.
func alpha() appcore.ServerStatus {
	return appcore.ServerStatus{URL: urlA, Name: "Alpha", Providers: []protocol.Provider{gitHub}, Provider: "gh"}
}

// beta lists two providers and none is chosen.
func beta() appcore.ServerStatus {
	return appcore.ServerStatus{URL: urlB, Name: "Beta", Providers: []protocol.Provider{gitHub, campus}}
}

func in(s appcore.ServerStatus) appcore.ServerStatus {
	s.LoggedIn, s.Email = true, "ada@example.org"
	return s
}

func up(s appcore.ServerStatus) appcore.ServerStatus {
	s = in(s)
	s.Connected, s.AssignedIP, s.Interface = true, "10.80.0.7/32", "Claimward"
	return s
}

// signedOut is a core with Alpha, nobody signed in.
func signedOut() *fakeCore { return newCore(alpha()) }

// signedIn is a core with Alpha, signed in.
func signedIn() *fakeCore { return newCore(in(alpha())) }

// twoServers is Alpha (signed in) and Beta (signed out).
func twoServers() *fakeCore { return newCore(in(alpha()), beta()) }

// newVM is a ViewModel over f whose results come back through q, refreshed
// once and settled.
func newVM(t *testing.T, f *fakeCore, opts ...Option) (*ViewModel, *Queue) {
	t.Helper()
	q := &Queue{}
	vm := NewViewModel(f, append([]Option{WithPost(q.Post)}, opts...)...)
	t.Cleanup(func() { vm.Stop(); vm.Wait() })
	vm.Refresh()
	settle(vm, q)
	return vm, q
}

// settle waits for every action to finish and runs what they posted, until
// nothing more is posted (a drained result can start another action).
func settle(vm *ViewModel, q *Queue) {
	for {
		vm.Wait()
		if !q.Pending() {
			return
		}
		q.Drain()
	}
}

// until drains the queue, waiting while an action is held, until cond holds.
func until(t *testing.T, q *Queue, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		q.Drain()
		if cond() {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("the awaited state never came")
		}
		time.Sleep(time.Millisecond)
	}
}

// run executes a command and settles.
func run(vm *ViewModel, q *Queue, execute func()) {
	execute()
	settle(vm, q)
}

var errBoom = errors.New("boom")
