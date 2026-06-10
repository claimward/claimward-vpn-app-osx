// Package appcore holds the Claimward macOS app's business logic, independent of
// the tray and the webview UI: OIDC login, enrollment against the server, and
// driving the privileged helper to bring the tunnel up and down.
package appcore

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/claimward/claimward-vpn-app-osx/internal/helperclient"
	"github.com/claimward/claimward-vpn-app-osx/internal/hproto"
	"github.com/claimward/claimward-vpn-client/pkg/client"
	"github.com/claimward/claimward-vpn-client/pkg/oidc"
	"github.com/claimward/claimward-vpn-client/pkg/protocol"
	"github.com/claimward/claimward-vpn-client/pkg/tokenstore"
	"github.com/claimward/claimward-vpn-client/pkg/wgkey"
)

// Core is the app's stateful service.
type Core struct {
	cfg    *Config
	api    *client.Client
	helper *helperclient.Client

	mu        sync.Mutex
	connected bool
	iface     string
	assigned  string
}

// New builds a Core from config.
func New(cfg *Config) *Core {
	return &Core{
		cfg:    cfg,
		api:    client.New(cfg.ServerURL),
		helper: helperclient.New(cfg.SocketPath),
	}
}

// Status is a snapshot for the UI and tray.
type Status struct {
	ConfigOK        bool   `json:"config_ok"`
	ConfigError     string `json:"config_error,omitempty"`
	LoggedIn        bool   `json:"logged_in"`
	Email           string `json:"email,omitempty"`
	HelperInstalled bool   `json:"helper_installed"`
	Connected       bool   `json:"connected"`
	Interface       string `json:"interface,omitempty"`
	AssignedIP      string `json:"assigned_ip,omitempty"`
	ServerURL       string `json:"server_url,omitempty"`
}

// Status returns the current state.
func (c *Core) Status() Status {
	st := Status{ServerURL: c.cfg.ServerURL}
	if err := c.cfg.Validate(); err != nil {
		st.ConfigError = err.Error()
	} else {
		st.ConfigOK = true
	}

	if sess, _ := tokenstore.Load(); sess != nil && sess.IDToken != "" {
		st.LoggedIn = true
		st.Email = emailFromIDToken(sess.IDToken)
	}

	st.HelperInstalled = c.helper.Available()
	if st.HelperInstalled {
		if hresp, err := c.helper.Status(); err == nil {
			st.Connected = hresp.Connected
			st.Interface = hresp.Interface
		}
	}

	c.mu.Lock()
	if st.Connected {
		st.AssignedIP = c.assigned
	}
	c.mu.Unlock()
	return st
}

// Login runs the interactive OIDC browser flow and persists the session.
func (c *Core) Login(ctx context.Context) error {
	if err := c.cfg.Validate(); err != nil {
		return err
	}
	toks, err := oidc.Login(ctx, oidc.Config{Issuer: c.cfg.OIDCIssuer, ClientID: c.cfg.OIDCClientID})
	if err != nil {
		return err
	}
	sess, _ := tokenstore.Load()
	if sess == nil {
		sess = &tokenstore.Session{}
	}
	sess.IDToken = toks.IDToken
	sess.AccessToken = toks.AccessToken
	sess.RefreshToken = toks.RefreshToken
	sess.Expiry = toks.Expiry
	if sess.WGPrivateKey == "" {
		pair, kerr := wgkey.Generate()
		if kerr != nil {
			return kerr
		}
		sess.WGPrivateKey = pair.Private.String()
	}
	return tokenstore.Save(sess)
}

// Connect enrolls the device and asks the helper to bring up the tunnel.
func (c *Core) Connect(ctx context.Context) error {
	if err := c.cfg.Validate(); err != nil {
		return err
	}
	sess, err := tokenstore.Load()
	if err != nil {
		return err
	}
	if sess == nil || sess.IDToken == "" {
		return fmt.Errorf("not signed in")
	}
	pair, err := wgkey.ParsePrivate(sess.WGPrivateKey)
	if err != nil {
		return fmt.Errorf("device key invalid, sign in again: %w", err)
	}

	host, _ := os.Hostname()
	resp, err := c.api.Enroll(ctx, sess.IDToken, pair.Public, protocol.DeviceInfo{
		Name: host, OS: "darwin", Platform: "app-osx",
	})
	if err != nil {
		return fmt.Errorf("enroll: %w", err)
	}

	spec := hproto.TunnelSpec{
		PrivateKey:      sess.WGPrivateKey,
		ServerPublicKey: resp.ServerPublicKey,
		Endpoint:        resp.Endpoint,
		Address:         resp.AssignedIP,
		AllowedIPs:      resp.AllowedIPs,
		DNS:             resp.DNS,
		MTU:             resp.MTU,
		Keepalive:       resp.PersistentKeepalive,
	}
	hresp, err := c.helper.Up(spec)
	if err != nil {
		return err
	}

	c.mu.Lock()
	c.connected = true
	c.iface = hresp.Interface
	c.assigned = resp.AssignedIP
	c.mu.Unlock()
	return nil
}

// Disconnect tears the tunnel down and deregisters the peer.
func (c *Core) Disconnect(ctx context.Context) error {
	_, derr := c.helper.Down()

	if sess, _ := tokenstore.Load(); sess != nil && sess.WGPrivateKey != "" {
		if pair, err := wgkey.ParsePrivate(sess.WGPrivateKey); err == nil {
			_ = c.api.Deregister(ctx, sess.IDToken, pair.Public)
		}
	}

	c.mu.Lock()
	c.connected = false
	c.iface = ""
	c.assigned = ""
	c.mu.Unlock()
	return derr
}

// Logout disconnects and clears the local session.
func (c *Core) Logout(ctx context.Context) error {
	_ = c.Disconnect(ctx)
	return tokenstore.Clear()
}

// emailFromIDToken extracts the "email" claim from a JWT without verifying it —
// for display only. Verification is the server's job.
func emailFromIDToken(idToken string) string {
	parts := strings.Split(idToken, ".")
	if len(parts) != 3 {
		return ""
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}
	var claims struct {
		Email string `json:"email"`
	}
	if json.Unmarshal(payload, &claims) != nil {
		return ""
	}
	return claims.Email
}
