package uiserver

import (
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/claimward/claimward-vpn-client/pkg/appcore"
	"github.com/claimward/claimward-vpn-client/pkg/hproto"
	"github.com/claimward/claimward-vpn-client/pkg/tokenstore"
	"github.com/claimward/claimward-vpn-client/pkg/wgkey"
)

// Audit 2026-11-23, finding 1: whoever holds the launch token (it travels in
// the Dashboard's argv, which another local user can read with `ps -axww`)
// can point the app at a socket of their own through POST /api/config, then
// POST /api/connect: the app must not hand that socket the person's bearer
// and WireGuard private key.
func TestConfigCannotRedirectTheHelperSocket(t *testing.T) {
	_, base, token := start(t)

	pair, err := wgkey.Generate()
	if err != nil {
		t.Fatal(err)
	}
	const bearer = "victim-bearer-token"
	if err := tokenstore.Save(&tokenstore.Session{Provider: "github", Bearer: bearer, BearerKind: "access_token",
		Expiry: time.Now().Add(time.Hour), WGPrivateKey: pair.Private.String()}); err != nil {
		t.Fatal(err)
	}

	// The attacker's socket (a short path: sun_path is 104 bytes on macOS).
	dir, err := os.MkdirTemp("", "cw")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	sock := filepath.Join(dir, "evil.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	got := make(chan hproto.Request, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		var req hproto.Request
		_ = json.NewDecoder(c).Decode(&req)
		got <- req
		_ = json.NewEncoder(c).Encode(hproto.Response{Error: "stolen"})
	}()

	cfg, _ := json.Marshal(appcore.Config{ServerURL: "https://vpn.example.org", Provider: "github",
		GitHubClientID: "x", SocketPath: sock})
	code, body := do(t, http.MethodPost, base+"/api/config", token, string(cfg))
	t.Logf("POST /api/config: %d %s", code, strings.TrimSpace(body))
	code, body = do(t, http.MethodPost, base+"/api/connect", token, "")
	t.Logf("POST /api/connect: %d %s", code, strings.TrimSpace(body))

	select {
	case req := <-got:
		if req.Connect != nil && (req.Connect.Bearer != "" || req.Connect.PrivateKey != "") {
			t.Fatalf("a socket chosen through /api/config received bearer=%q private_key=%q", req.Connect.Bearer, req.Connect.PrivateKey)
		}
	case <-time.After(2 * time.Second):
	}
}
