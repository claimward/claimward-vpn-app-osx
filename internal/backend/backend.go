// SPDX-License-Identifier: BSD-3-Clause
//
// Package backend is what the app adds to claimward-vpn-client's appcore.Core:
// the helper's approval of a server (the helper trusts a server only after the
// person said so, through the system's own authorization dialog), a preview of
// a server before it is added, and the helper's view of the tunnels, cached so
// that the UI thread never waits on a socket.
//
// It implements appvm.Service by wrapping a Core, and answers a helper's
// "not approved" refusal to Connect itself: it obtains the system's
// authorization naming the server, has the helper approve, and connects again.
package backend

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/claimward/claimward-vpn-client/pkg/appcore"
	"github.com/claimward/claimward-vpn-client/pkg/appvm"
	"github.com/claimward/claimward-vpn-client/pkg/discovery"
	"github.com/claimward/claimward-vpn-client/pkg/helperclient"
	"github.com/claimward/claimward-vpn-client/pkg/hproto"
	"github.com/claimward/claimward-vpn-client/pkg/protocol"
)

// Preview is what a server says about itself, read before it is added.
type Preview struct {
	URL, Name string
	// Providers are the display names of the identity providers it accepts.
	Providers []string
	// Exclusive: it refuses to share the machine with any other server.
	Exclusive bool
	// Logo is its SVG (already checked by the client), or nil.
	Logo []byte
}

// Helper is the part of helperclient.Client the backend uses.
type Helper interface {
	Available() bool
	Status() (*hproto.Response, error)
	Approved() (*hproto.Response, error)
	Approve(server string, authorization []byte) (*hproto.Response, error)
	Revoke(server string) (*hproto.Response, error)
}

var _ Helper = (*helperclient.Client)(nil)

// Discovery is the part of discovery.Client the backend uses.
type Discovery interface {
	Get(ctx context.Context, server string) (*protocol.Configuration, error)
	Logo(ctx context.Context, server string, cfg *protocol.Configuration) ([]byte, error)
}

var _ Discovery = (*discovery.Client)(nil)

// Backend wraps a Core.
type Backend struct {
	appvm.Service
	helper Helper
	disc   Discovery
	// obtain asks the system for the authorization to approve a server; prompt
	// names the server in the system's dialog (macauth.Obtain).
	obtain func(prompt string) ([]byte, error)
	now    func() time.Time

	mu       sync.Mutex
	approved map[string]bool
	leases   map[string]time.Time
}

// New wraps core. helper is the helper's socket client, disc the discovery
// client, obtain the system authorization (macauth.Obtain).
func New(core appvm.Service, helper Helper, disc Discovery, obtain func(string) ([]byte, error)) *Backend {
	return &Backend{
		Service: core, helper: helper, disc: disc, obtain: obtain, now: time.Now,
		approved: map[string]bool{}, leases: map[string]time.Time{},
	}
}

// Status is the Core's, with the helper's approvals and leases cached on the
// way for Approved and LeaseExpires.
func (b *Backend) Status() appcore.Status {
	st := b.Service.Status()
	if st.HelperInstalled {
		b.refresh()
	}
	return st
}

func (b *Backend) refresh() {
	approved := map[string]bool{}
	if r, err := b.helper.Approved(); err == nil {
		for _, a := range r.Approved {
			approved[a.ServerURL] = true
		}
	}
	leases := map[string]time.Time{}
	if r, err := b.helper.Status(); err == nil {
		for _, t := range r.Tunnels {
			if t.LeaseExpiresAt != nil {
				leases[t.ServerURL] = *t.LeaseExpiresAt
			}
		}
	}
	b.mu.Lock()
	b.approved, b.leases = approved, leases
	b.mu.Unlock()
}

// Approved says whether the helper trusts the server (as of the last Status).
func (b *Backend) Approved(server string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.approved[server]
}

// LeaseExpires is when the server's lease on the tunnel ends, zero when there
// is none (as of the last Status).
func (b *Backend) LeaseExpires(server string) time.Time {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.leases[server]
}

// Host is the server's host, the URL itself when it has none.
func Host(raw string) string {
	if u, err := url.Parse(raw); err == nil && u.Host != "" {
		return u.Host
	}
	return raw
}

// ErrNotApproved is what Connect returns when the person did not approve.
var ErrNotApproved = errors.New("not approved")

// Connect connects, approving the server first when the helper asks for it.
func (b *Backend) Connect(ctx context.Context, server string) error {
	err := b.Service.Connect(ctx, server)
	var na *helperclient.NotApprovedError
	if !errors.As(err, &na) {
		return explain(err)
	}
	if aerr := b.approve(server, na.Changed); aerr != nil {
		return aerr
	}
	return explain(b.Service.Connect(ctx, server))
}

// Disconnect is the Core's, with the helper's refusals told readably.
func (b *Backend) Disconnect(ctx context.Context, server string) error {
	return explain(b.Service.Disconnect(ctx, server))
}

// approve obtains the system's authorization, naming the server, and has the
// helper trust it.
func (b *Backend) approve(server string, changed bool) error {
	host := Host(server)
	prompt := fmt.Sprintf("Claimward wants to trust the VPN server %s and let it set up a tunnel on this Mac.", host)
	if changed {
		prompt = fmt.Sprintf("The VPN server %s has changed its identity since you approved it. Claimward wants to trust it again.", host)
	}
	auth, err := b.obtain(prompt)
	if err != nil {
		return fmt.Errorf("%w: you did not approve %s (%v)", ErrNotApproved, host, err)
	}
	if _, err := b.helper.Approve(server, auth); err != nil {
		if errors.Is(err, helperclient.ErrApprovalDenied) {
			return fmt.Errorf("%w: the system refused to approve %s", ErrNotApproved, host)
		}
		var ref *Refusal
		if told := explain(err); errors.As(told, &ref) {
			return told
		}
		return fmt.Errorf("approving %s failed: %w", host, err)
	}
	b.mu.Lock()
	b.approved[server] = true
	b.mu.Unlock()
	return nil
}

// Revoke makes the helper forget the server.
func (b *Backend) Revoke(_ context.Context, server string) error {
	if _, err := b.helper.Revoke(server); err != nil {
		return explain(err)
	}
	b.mu.Lock()
	delete(b.approved, server)
	b.mu.Unlock()
	return nil
}

// Preview reads the server's discovery document and logo without adding it.
func (b *Backend) Preview(ctx context.Context, raw string) (Preview, error) {
	cfg, err := b.disc.Get(ctx, raw)
	if err != nil {
		return Preview{}, err
	}
	p := Preview{URL: raw, Name: cfg.ServerName, Exclusive: cfg.Exclusive}
	if p.Name == "" {
		p.Name = Host(raw)
	}
	for _, pr := range cfg.Providers {
		p.Providers = append(p.Providers, pr.Name)
	}
	if cfg.Logo != nil {
		if svg, lerr := b.disc.Logo(ctx, raw, cfg); lerr == nil {
			p.Logo = svg
		}
	}
	return p, nil
}
