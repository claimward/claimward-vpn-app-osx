// SPDX-License-Identifier: BSD-3-Clause

package backend

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/claimward/claimward-vpn-client/pkg/appcore"
	"github.com/claimward/claimward-vpn-client/pkg/appvm"
	"github.com/claimward/claimward-vpn-client/pkg/helperclient"
	"github.com/claimward/claimward-vpn-client/pkg/hproto"
	"github.com/claimward/claimward-vpn-client/pkg/protocol"
)

// fakeService stands in for appcore.Core: Status is st, Connect answers each
// call from connectErrs in turn (nil when they run out).
type fakeService struct {
	appvm.Service // every other method would panic: the backend must not use it
	st            appcore.Status
	connectErrs   []error
	connects      []string
}

func (f *fakeService) Status() appcore.Status { return f.st }

func (f *fakeService) Connect(_ context.Context, server string) error {
	f.connects = append(f.connects, server)
	if len(f.connects) <= len(f.connectErrs) {
		return f.connectErrs[len(f.connects)-1]
	}
	return nil
}

// fakeHelper records what the backend asks of the helper.
type fakeHelper struct {
	approved               *hproto.Response
	approvedErr, statusErr error
	status                 *hproto.Response
	approveErr, revokeErr  error
	approves               []string
	auth                   []byte
	revokes                []string
}

func (h *fakeHelper) Available() bool { return true }
func (h *fakeHelper) Status() (*hproto.Response, error) {
	return h.status, h.statusErr
}
func (h *fakeHelper) Approved() (*hproto.Response, error) { return h.approved, h.approvedErr }
func (h *fakeHelper) Approve(server string, auth []byte) (*hproto.Response, error) {
	h.approves = append(h.approves, server)
	h.auth = auth
	return &hproto.Response{OK: true}, h.approveErr
}
func (h *fakeHelper) Revoke(server string) (*hproto.Response, error) {
	h.revokes = append(h.revokes, server)
	return &hproto.Response{OK: true}, h.revokeErr
}

// fakeDiscovery answers Get and Logo from fields.
type fakeDiscovery struct {
	cfg     *protocol.Configuration
	getErr  error
	logo    []byte
	logoErr error
	asked   []string
}

func (d *fakeDiscovery) Get(_ context.Context, server string) (*protocol.Configuration, error) {
	d.asked = append(d.asked, server)
	return d.cfg, d.getErr
}

func (d *fakeDiscovery) Logo(_ context.Context, server string, _ *protocol.Configuration) ([]byte, error) {
	d.asked = append(d.asked, "logo "+server)
	return d.logo, d.logoErr
}

const srv = "https://vpn.example.org"

func newBackend(svc *fakeService, h *fakeHelper, d *fakeDiscovery, obtain func(string) ([]byte, error)) *Backend {
	return New(svc, h, d, obtain)
}

func TestStatusCachesTheHelpersApprovalsAndLeases(t *testing.T) {
	exp := time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)
	h := &fakeHelper{
		approved: &hproto.Response{Approved: []hproto.Approved{{ServerURL: srv}}},
		status: &hproto.Response{Tunnels: []hproto.Tunnel{
			{ServerURL: srv, LeaseExpiresAt: &exp},
			{ServerURL: "https://other.example.org"}, // no lease
		}},
	}
	svc := &fakeService{st: appcore.Status{HelperInstalled: true}}
	b := newBackend(svc, h, nil, nil)
	if b.Approved(srv) || !b.LeaseExpires(srv).IsZero() {
		t.Fatal("approved or leased before any status")
	}
	if got := b.Status(); !got.HelperInstalled {
		t.Fatalf("status %+v is not the service's", got)
	}
	if !b.Approved(srv) || b.Approved("https://other.example.org") {
		t.Error("approvals not cached")
	}
	if !b.LeaseExpires(srv).Equal(exp) || !b.LeaseExpires("https://other.example.org").IsZero() {
		t.Error("leases not cached")
	}

	// The helper no longer answers: what was cached is dropped, not kept stale.
	h.approvedErr, h.statusErr = errors.New("down"), errors.New("down")
	b.Status()
	if b.Approved(srv) || !b.LeaseExpires(srv).IsZero() {
		t.Error("a helper that does not answer left the cache as it was")
	}
}

func TestStatusLeavesTheCacheAloneWithoutAHelper(t *testing.T) {
	h := &fakeHelper{approved: &hproto.Response{Approved: []hproto.Approved{{ServerURL: srv}}}, status: &hproto.Response{}}
	svc := &fakeService{st: appcore.Status{HelperInstalled: true}}
	b := newBackend(svc, h, nil, nil)
	b.Status()
	svc.st.HelperInstalled = false
	h.approved = nil // would panic if asked
	b.Status()
	if !b.Approved(srv) {
		t.Error("a status without a helper cleared the approvals")
	}
}

func TestHost(t *testing.T) {
	for in, want := range map[string]string{
		srv:                              "vpn.example.org",
		"https://vpn.example.org:8443/x": "vpn.example.org:8443",
		"vpn.example.org":                "vpn.example.org", // no scheme: no host
		"%zz":                            "%zz",             // does not parse
	} {
		if got := Host(in); got != want {
			t.Errorf("Host(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestConnectNeedsNoApprovalWhenTheHelperTrustsTheServer(t *testing.T) {
	svc := &fakeService{}
	b := newBackend(svc, &fakeHelper{}, nil, func(string) ([]byte, error) { t.Fatal("asked the system"); return nil, nil })
	if err := b.Connect(context.Background(), srv); err != nil || len(svc.connects) != 1 {
		t.Fatalf("err %v, connects %v", err, svc.connects)
	}
	// Any other failure is the service's, untouched.
	boom := errors.New("boom")
	svc.connects, svc.connectErrs = nil, []error{boom}
	if err := b.Connect(context.Background(), srv); err != boom {
		t.Fatalf("err %v, want %v", err, boom)
	}
}

func TestConnectApprovesTheServerThenConnectsOnce(t *testing.T) {
	svc := &fakeService{connectErrs: []error{&helperclient.NotApprovedError{Server: srv, Message: "server not approved"}}}
	h := &fakeHelper{}
	var prompts []string
	b := newBackend(svc, h, nil, func(p string) ([]byte, error) { prompts = append(prompts, p); return []byte("auth"), nil })
	if err := b.Connect(context.Background(), srv); err != nil {
		t.Fatal(err)
	}
	if len(svc.connects) != 2 || len(h.approves) != 1 || h.approves[0] != srv || string(h.auth) != "auth" {
		t.Fatalf("connects %v approves %v auth %q", svc.connects, h.approves, h.auth)
	}
	if len(prompts) != 1 || !strings.Contains(prompts[0], "vpn.example.org") || strings.Contains(prompts[0], "changed") {
		t.Fatalf("prompt %q does not name the host", prompts)
	}
	if !b.Approved(srv) {
		t.Error("the approval is not remembered")
	}
	if err := b.Revoke(context.Background(), srv); err != nil || b.Approved(srv) || len(h.revokes) != 1 {
		t.Errorf("revoke: err %v approved %v revokes %v", err, b.Approved(srv), h.revokes)
	}
}

func TestAChangedIdentityIsSaidInThePrompt(t *testing.T) {
	svc := &fakeService{connectErrs: []error{fmt.Errorf("wrapped: %w", &helperclient.NotApprovedError{Server: srv, Changed: true})}}
	var prompt string
	b := newBackend(svc, &fakeHelper{}, nil, func(p string) ([]byte, error) { prompt = p; return nil, nil })
	if err := b.Connect(context.Background(), srv); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, "changed its identity") || !strings.Contains(prompt, "vpn.example.org") {
		t.Fatalf("prompt %q", prompt)
	}
}

func TestApprovalFailures(t *testing.T) {
	na := &helperclient.NotApprovedError{Server: srv}
	cases := []struct {
		name       string
		obtainErr  error
		approveErr error
		want       string
		notApprov  bool
	}{
		{"the person said no", errors.New("cancelled"), nil, "you did not approve vpn.example.org (cancelled)", true},
		{"the system refused", nil, fmt.Errorf("%w: nope", helperclient.ErrApprovalDenied), "the system refused to approve vpn.example.org", true},
		{"the helper failed", nil, errors.New("socket closed"), "approving vpn.example.org failed: socket closed", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			svc := &fakeService{connectErrs: []error{na}}
			h := &fakeHelper{approveErr: c.approveErr}
			b := newBackend(svc, h, nil, func(string) ([]byte, error) { return nil, c.obtainErr })
			err := b.Connect(context.Background(), srv)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("err %v, want %q", err, c.want)
			}
			if errors.Is(err, ErrNotApproved) != c.notApprov {
				t.Errorf("ErrNotApproved = %v", errors.Is(err, ErrNotApproved))
			}
			if len(svc.connects) != 1 || b.Approved(srv) {
				t.Errorf("connected again (%v) or remembered an approval", svc.connects)
			}
		})
	}
}

func TestRevokeFailureKeepsTheApproval(t *testing.T) {
	h := &fakeHelper{approved: &hproto.Response{Approved: []hproto.Approved{{ServerURL: srv}}}, status: &hproto.Response{}}
	b := newBackend(&fakeService{st: appcore.Status{HelperInstalled: true}}, h, nil, nil)
	b.Status()
	h.revokeErr = errors.New("refused")
	if err := b.Revoke(context.Background(), srv); err == nil || !b.Approved(srv) {
		t.Fatalf("err %v approved %v", err, b.Approved(srv))
	}
}

func TestPreview(t *testing.T) {
	cfg := &protocol.Configuration{
		ServerName: "Lab VPN", Exclusive: true,
		Providers: []protocol.Provider{{ID: "gh", Name: "GitHub"}, {ID: "o", Name: "Campus"}},
		Logo:      &protocol.Logo{Path: protocol.PathWellKnownLogo},
	}
	d := &fakeDiscovery{cfg: cfg, logo: []byte("<svg/>")}
	b := newBackend(nil, nil, d, nil)
	p, err := b.Preview(context.Background(), srv)
	if err != nil {
		t.Fatal(err)
	}
	if p.URL != srv || p.Name != "Lab VPN" || !p.Exclusive || strings.Join(p.Providers, "|") != "GitHub|Campus" || string(p.Logo) != "<svg/>" {
		t.Fatalf("preview %+v", p)
	}

	// A logo that cannot be fetched leaves none; no name falls back to the host;
	// no logo document asks for no logo.
	d.logoErr = errors.New("digest mismatch")
	p, _ = b.Preview(context.Background(), srv)
	if p.Logo != nil {
		t.Errorf("logo %q despite the error", p.Logo)
	}
	d.cfg = &protocol.Configuration{}
	d.asked = nil
	p, err = b.Preview(context.Background(), srv)
	if err != nil || p.Name != "vpn.example.org" || len(d.asked) != 1 || len(p.Providers) != 0 {
		t.Fatalf("preview %+v err %v asked %v", p, err, d.asked)
	}

	d.getErr = errors.New("not a Claimward server")
	if _, err = b.Preview(context.Background(), srv); err == nil {
		t.Fatal("a failed discovery was previewed")
	}
}
