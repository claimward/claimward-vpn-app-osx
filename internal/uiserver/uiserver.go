// Package uiserver serves the embedded Svelte single-page app and a small JSON
// API on a loopback-only listener. The webview process points at the returned
// URL; the SPA drives the app entirely through this API.
//
// The API is guarded by a per-launch random token (passed to the webview in the
// URL) so other local processes cannot drive the tunnel. Static assets are
// served unguarded.
package uiserver

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/claimward/claimward-vpn-client/pkg/appcore"
	"github.com/claimward/claimward-vpn-client/pkg/browser"
)

//go:embed all:dist
var distFS embed.FS

// Server is the running loopback UI server.
type Server struct {
	core  *appcore.Core
	token string
	http  *http.Server
	url   string
}

// Start launches the server on 127.0.0.1:<random> and returns it. Call URL() to
// get the address (including the access token) to open in the webview.
func Start(core *appcore.Core) (*Server, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	s := &Server{core: core, token: randToken()}

	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		return nil, err
	}

	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.FS(sub)))
	mux.HandleFunc("/api/status", s.guard(s.handleStatus))
	mux.HandleFunc("/api/config", s.guard(s.handleConfig))
	mux.HandleFunc("/api/open", s.guard(s.handleOpen))
	mux.HandleFunc("/api/login", s.guard(s.handleLogin))
	mux.HandleFunc("/api/connect", s.guard(s.handleConnect))
	mux.HandleFunc("/api/disconnect", s.guard(s.handleDisconnect))
	mux.HandleFunc("/api/logout", s.guard(s.handleLogout))
	mux.HandleFunc("/api/tenants", s.guard(s.handleTenants))
	mux.HandleFunc("/api/tenant", s.guard(s.handleTenant))

	s.http = &http.Server{Handler: mux}
	s.url = fmt.Sprintf("http://%s/?t=%s", ln.Addr().String(), s.token)
	go s.http.Serve(ln) //nolint:errcheck
	return s, nil
}

// URL returns the loopback URL (with token) to open in the webview.
func (s *Server) URL() string { return s.url }

// Close stops the server.
func (s *Server) Close() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = s.http.Shutdown(ctx)
}

func (s *Server) guard(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tok := r.Header.Get("X-Claimward-Token")
		if tok == "" {
			tok = r.URL.Query().Get("t")
		}
		if subtle.ConstantTimeCompare([]byte(tok), []byte(s.token)) != 1 {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		h(w, r)
	}
}

func (s *Server) handleStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.core.Status())
}

// handleOpen opens an http(s) URL in the system browser via the Go process —
// reliable even when the webview won't open target=_blank links externally.
func (s *Server) handleOpen(w http.ResponseWriter, r *http.Request) {
	u := r.URL.Query().Get("url")
	if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
		http.Error(w, "only http(s) URLs allowed", http.StatusBadRequest)
		return
	}
	if err := browser.Open(u); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleConfig returns the current configuration (GET) or saves a new one (POST).
func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		var cfg appcore.Config
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			writeErr(w, err)
			return
		}
		if err := s.core.UpdateConfig(cfg); err != nil {
			writeErr(w, err)
			return
		}
	}
	writeJSON(w, s.core.Config())
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancel()
	if err := s.core.Login(ctx); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, s.core.Status())
}

func (s *Server) handleConnect(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	if err := s.core.Connect(ctx); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, s.core.Status())
}

func (s *Server) handleDisconnect(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if err := s.core.Disconnect(ctx); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, s.core.Status())
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if err := s.core.Logout(ctx); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, s.core.Status())
}

// handleTenants asks the server which tenants the person may connect to.
func (s *Server) handleTenants(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	if _, err := s.core.Tenants(ctx); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, s.core.Status())
}

// handleTenant records the tenant chosen for this session.
func (s *Server) handleTenant(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST", http.StatusMethodNotAllowed)
		return
	}
	var in struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4<<10)).Decode(&in); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if err := s.core.SetTenant(in.ID); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, s.core.Status())
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadGateway)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}

func randToken() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}
