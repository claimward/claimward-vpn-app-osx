// SPDX-License-Identifier: BSD-3-Clause

package backend

import (
	"strings"

	"github.com/claimward/claimward-vpn-client/pkg/hproto"
)

// Refusal is a helper refusal the person can act on, told in the app's own
// words. The helper's text (Cause) stays reachable through errors.Unwrap for
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

// The client turns a refusal into the text "helper: <message>" and drops the
// code, so Connect and Disconnect, which go through the Core, recognise these
// by the helper's own messages (pkg/helper, client v0.12.2).
var refusalMarker = map[string]string{
	"that tunnel belongs to another user of this machine": hproto.ErrNotOwner,
	"cannot identify the caller":                          hproto.ErrCallerUnidentified,
}

// Error is the message for the code.
func (r *Refusal) Error() string { return refusalText[r.Code] }

// Unwrap is the helper's own error.
func (r *Refusal) Unwrap() error { return r.Cause }

// explainCode is err as a Refusal when the helper answered with one of the
// refusals above, err itself otherwise.
func explainCode(code string, err error) error {
	if _, known := refusalText[code]; err == nil || !known {
		return err
	}
	return &Refusal{Code: code, Cause: err}
}

// explainText is the same for an error that only carries the helper's text.
func explainText(err error) error {
	if err == nil {
		return nil
	}
	for marker, code := range refusalMarker {
		if strings.Contains(err.Error(), marker) {
			return explainCode(code, err)
		}
	}
	return err
}

// explain is err as a Refusal, from the response's code when the helper
// answered, from the text otherwise.
func explain(resp *hproto.Response, err error) error {
	if resp != nil {
		if _, known := refusalText[resp.ErrorCode]; known {
			return explainCode(resp.ErrorCode, err)
		}
	}
	return explainText(err)
}
