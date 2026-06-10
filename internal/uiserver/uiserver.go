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
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"time"

	"github.com/claimward/claimward-vpn-app-osx/internal/appcore"
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
	mux.HandleFunc("/api/login", s.guard(s.handleLogin))
	mux.HandleFunc("/api/connect", s.guard(s.handleConnect))
	mux.HandleFunc("/api/disconnect", s.guard(s.handleDisconnect))
	mux.HandleFunc("/api/logout", s.guard(s.handleLogout))

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
		if tok != s.token {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		h(w, r)
	}
}

func (s *Server) handleStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.core.Status())
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
