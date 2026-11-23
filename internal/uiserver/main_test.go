package uiserver

import (
	"net/http"
	"os"
	"testing"
)

// TestMain keeps the tests off the real Keychain: they run with the 0600 file
// session opt-in, in a temporary HOME (see start).
func TestMain(m *testing.M) {
	os.Setenv("CLAIMWARD_ALLOW_PLAINTEXT_SESSION", "1")
	os.Exit(m.Run())
}

// The token is accepted in the X-Claimward-Token header only: in a query
// string it ends up in logs, history and Referer headers.
func TestTheTokenIsNotAcceptedInTheQueryString(t *testing.T) {
	_, base, token := start(t)
	if code, _ := do(t, http.MethodGet, base+"/api/status", token, ""); code != http.StatusOK {
		t.Fatalf("header token: %d", code)
	}
	if code, _ := do(t, http.MethodGet, base+"/api/status?t="+token, "", ""); code != http.StatusForbidden {
		t.Errorf("query token: %d, want 403", code)
	}
}
