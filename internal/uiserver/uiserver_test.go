package uiserver

import (
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/claimward/claimward-vpn-client/pkg/appcore"
)

func start(t *testing.T) (*Server, string, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	core := appcore.New(&appcore.Config{ServerURL: "https://vpn.example.org", Provider: "oidc",
		OIDCIssuer: "https://login.example.org", OIDCClientID: "claimward", SocketPath: filepath.Join(home, "none.sock")})
	s, err := Start(core)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	u, _ := url.Parse(s.URL())
	return s, u.Scheme + "://" + u.Host, u.Query().Get("t")
}

func do(t *testing.T, method, u, token, body string) (int, string) {
	t.Helper()
	req, _ := http.NewRequest(method, u, strings.NewReader(body))
	if token != "" {
		req.Header.Set("X-Claimward-Token", token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(b)
}

// The tenant endpoints are behind the launch token like the rest, and say
// why they cannot answer.
func TestTheTenantEndpoints(t *testing.T) {
	_, base, token := start(t)
	if code, _ := do(t, http.MethodGet, base+"/api/tenants", "", ""); code != http.StatusForbidden {
		t.Errorf("no token: %d", code)
	}
	if code, _ := do(t, http.MethodGet, base+"/api/tenants", token+"x", ""); code != http.StatusForbidden {
		t.Errorf("a wrong token: %d", code)
	}
	if code, body := do(t, http.MethodGet, base+"/api/tenants", token, ""); code != http.StatusBadGateway || !strings.Contains(body, "not signed in") {
		t.Errorf("tenants while signed out: %d %s", code, body)
	}
	if code, body := do(t, http.MethodPost, base+"/api/tenant", token, `{"id": "physics"}`); code != http.StatusBadGateway || !strings.Contains(body, "not a tenant you were offered") {
		t.Errorf("a tenant nobody offered: %d %s", code, body)
	}
	if code, _ := do(t, http.MethodPost, base+"/api/tenant", token, `not json`); code != http.StatusBadRequest {
		t.Errorf("a body that is not JSON: %d", code)
	}
	if code, _ := do(t, http.MethodGet, base+"/api/tenant", token, ""); code != http.StatusMethodNotAllowed {
		t.Errorf("GET /api/tenant: %d", code)
	}
}
