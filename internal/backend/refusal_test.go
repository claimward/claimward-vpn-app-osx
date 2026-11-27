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

// The helper's own texts, as the client composes them ("helper: " + message).
var (
	errNotOwned     = errors.New("helper: that tunnel belongs to another user of this machine")
	errUnidentified = errors.New("helper: cannot identify the caller: no peer credentials")
)

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
	if msg == "" || strings.Contains(msg, "helper:") || strings.Contains(msg, code) {
		t.Errorf("message %q is not readable", msg)
	}
}

func TestConnectAndDisconnectTellTheHelpersRefusalsReadably(t *testing.T) {
	for _, tc := range []struct {
		cause error
		code  string
	}{
		{errNotOwned, hproto.ErrNotOwner},
		{errUnidentified, hproto.ErrCallerUnidentified},
	} {
		svc := &fakeService{connectErrs: []error{tc.cause}, disconnectErr: tc.cause}
		b := newBackend(svc, &fakeHelper{}, nil, nil)
		wantRefusal(t, b.Connect(context.Background(), srv), tc.cause, tc.code)
		wantRefusal(t, b.Disconnect(context.Background(), srv), tc.cause, tc.code)
	}
	if got := (&Refusal{Code: hproto.ErrNotOwner}).Error(); !strings.Contains(got, "another user") {
		t.Errorf("not owned: %q", got)
	}
	if got := (&Refusal{Code: hproto.ErrCallerUnidentified}).Error(); !strings.Contains(got, "which user") {
		t.Errorf("not identified: %q", got)
	}
}

func TestOtherFailuresAreLeftAlone(t *testing.T) {
	boom := errors.New("boom")
	svc := &fakeService{disconnectErr: boom}
	b := newBackend(svc, &fakeHelper{}, nil, nil)
	if err := b.Disconnect(context.Background(), srv); err != boom {
		t.Errorf("Disconnect: %v", err)
	}
	if err := b.Disconnect(context.Background(), srv); err != boom {
		t.Errorf("Disconnect again: %v", err)
	}
	svc.disconnectErr = nil
	if err := b.Disconnect(context.Background(), srv); err != nil {
		t.Errorf("Disconnect without error: %v", err)
	}
	// No error is no refusal: asserted, so that a crash cannot pass for it.
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("explainText(nil) panicked: %v", r)
			}
		}()
		if err := explainText(nil); err != nil {
			t.Errorf("explainText(nil) = %v", err)
		}
	}()
	// A known code with no error is not a failure.
	if err := explainCode(hproto.ErrNotOwner, nil); err != nil {
		t.Errorf("explainCode(nil) = %v", err)
	}
	// A response with another code falls back to the text, then to the error.
	if err := explain(&hproto.Response{ErrorCode: hproto.ErrExclusive}, boom); err != boom {
		t.Errorf("explain(exclusive) = %v", err)
	}
	wantRefusal(t, explain(&hproto.Response{}, errNotOwned), errNotOwned, hproto.ErrNotOwner)
}

func TestRevokeTellsTheCodeReadably(t *testing.T) {
	for _, code := range []string{hproto.ErrNotOwner, hproto.ErrCallerUnidentified} {
		cause := errors.New("whatever text the helper sent")
		h := &fakeHelper{revokeErr: cause, code: code}
		b := newBackend(&fakeService{}, h, nil, nil)
		wantRefusal(t, b.Revoke(context.Background(), srv), cause, code)
	}
	// Without a code it is the helper's error as it came.
	boom := errors.New("boom")
	b := newBackend(&fakeService{}, &fakeHelper{revokeErr: boom}, nil, nil)
	if err := b.Revoke(context.Background(), srv); err != boom {
		t.Errorf("Revoke = %v", err)
	}
}

func TestApprovingTellsTheCodeReadably(t *testing.T) {
	cause := errors.New("whatever text the helper sent")
	svc := &fakeService{connectErrs: []error{&helperclient.NotApprovedError{Server: srv, Message: "not approved"}}}
	h := &fakeHelper{approveErr: cause, code: hproto.ErrCallerUnidentified}
	b := newBackend(svc, h, nil, func(string) ([]byte, error) { return []byte("auth"), nil })
	wantRefusal(t, b.Connect(context.Background(), srv), cause, hproto.ErrCallerUnidentified)
}
