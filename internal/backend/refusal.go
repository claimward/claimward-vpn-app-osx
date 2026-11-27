// SPDX-License-Identifier: BSD-3-Clause

package backend

import (
	"github.com/claimward/claimward-vpn-client/pkg/helperclient"
	"github.com/claimward/claimward-vpn-client/pkg/hproto"
)

// Refusal is a helper refusal the person can act on, told in the app's own
// words. The helper's error (Cause) stays reachable through errors.Unwrap for
// the log; the window shows Error.
type Refusal struct {
	// Code is the helper's error_code.
	Code  string
	Cause error
}

// The messages, by error_code.
var refusalText = map[string]string{
	hproto.ErrNotOwner: "This tunnel was started by another user of this Mac, " +
		"so only that user can change or close it.",
	hproto.ErrCallerUnidentified: "The helper could not tell which user is asking, " +
		"so it refused. Quit Claimward, open it again and retry.",
}

// Error is the message for the code.
func (r *Refusal) Error() string { return refusalText[r.Code] }

// Unwrap is the helper's own error.
func (r *Refusal) Unwrap() error { return r.Cause }

// explain is err as a Refusal when the helper refused with one of the codes
// above (read with helperclient.CodeOf), err itself otherwise.
func explain(err error) error {
	if err == nil {
		return nil
	}
	code := helperclient.CodeOf(err)
	if _, known := refusalText[code]; !known {
		return err
	}
	return &Refusal{Code: code, Cause: err}
}
