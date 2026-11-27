// SPDX-License-Identifier: BSD-3-Clause

package backend

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/claimward/claimward-vpn-client/pkg/helperclient"
	"github.com/claimward/claimward-vpn-client/pkg/hproto"
)

// refused is what the client returns for a helper refusal with the code.
func refused(code string) error {
	return &helperclient.RefusalError{Code: code, Message: "whatever the helper said"}
}

func wantRefusal(t *testing.T, err, cause error, code string) {
	t.Helper()
	var r *Refusal
	if !errors.As(err, &r) || r.Code != code {
		t.Fatalf("err %v is not a refusal with code %q", err, code)
	}
	if !errors.Is(err, cause) {
		t.Errorf("the helper's own error is lost from %v", err)
	}
	msg := err.Error()
	if msg == "" || strings.HasPrefix(msg, "helper:") || strings.Contains(msg, code) {
		t.Errorf("message %q is not readable", msg)
	}
}

var codes = []string{hproto.ErrNotOwner, hproto.ErrCallerUnidentified}

func TestConnectAndDisconnectTellTheHelpersRefusalsReadably(t *testing.T) {
	for _, code := range codes {
		cause := refused(code)
		svc := &fakeService{connectErrs: []error{cause}, disconnectErr: cause}
		b := newBackend(svc, &fakeHelper{}, nil, nil)
		wantRefusal(t, b.Connect(context.Background(), srv), cause, code)
		wantRefusal(t, b.Disconnect(context.Background(), srv), cause, code)
	}
	if got := (&Refusal{Code: hproto.ErrNotOwner}).Error(); !strings.Contains(got, "another user") {
		t.Errorf("not owned: %q", got)
	}
	if got := (&Refusal{Code: hproto.ErrCallerUnidentified}).Error(); !strings.Contains(got, "which user") {
		t.Errorf("not identified: %q", got)
	}
}

func TestTheWordsOfTheHelperDecideNothing(t *testing.T) {
	// The same words without the code are not a refusal this app tells apart.
	same := errors.New("helper: that tunnel belongs to another user of this machine")
	svc := &fakeService{connectErrs: []error{same}, disconnectErr: same}
	b := newBackend(svc, &fakeHelper{}, nil, nil)
	if err := b.Connect(context.Background(), srv); err != same {
		t.Errorf("Connect = %v", err)
	}
	if err := b.Disconnect(context.Background(), srv); err != same {
		t.Errorf("Disconnect = %v", err)
	}
	// Another code is left as it came.
	other := refused(hproto.ErrExclusive)
	svc.disconnectErr = other
	if err := b.Disconnect(context.Background(), srv); err != other {
		t.Errorf("Disconnect(exclusive) = %v", err)
	}
	// No error is no refusal: asserted, so that a crash cannot pass for it.
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("explain(nil) panicked: %v", r)
			}
		}()
		if err := explain(nil); err != nil {
			t.Errorf("explain(nil) = %v", err)
		}
	}()
}

func TestRevokeTellsTheCodeReadably(t *testing.T) {
	for _, code := range codes {
		cause := refused(code)
		b := newBackend(&fakeService{}, &fakeHelper{revokeErr: cause}, nil, nil)
		wantRefusal(t, b.Revoke(context.Background(), srv), cause, code)
	}
	boom := errors.New("boom")
	b := newBackend(&fakeService{}, &fakeHelper{revokeErr: boom}, nil, nil)
	if err := b.Revoke(context.Background(), srv); err != boom {
		t.Errorf("Revoke = %v", err)
	}
}

func TestApprovingTellsTheCodeReadably(t *testing.T) {
	cause := refused(hproto.ErrCallerUnidentified)
	svc := &fakeService{connectErrs: []error{&helperclient.NotApprovedError{Server: srv, Message: "not approved"}}}
	h := &fakeHelper{approveErr: cause}
	b := newBackend(svc, h, nil, func(string) ([]byte, error) { return []byte("auth"), nil })
	wantRefusal(t, b.Connect(context.Background(), srv), cause, hproto.ErrCallerUnidentified)
}
