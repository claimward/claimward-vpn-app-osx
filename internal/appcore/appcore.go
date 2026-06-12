// Package appcore holds the Claimward macOS app's business logic, independent of
// the tray and the webview UI: provider login, enrollment against the server,
// and driving the privileged helper to bring the tunnel up and down.
package appcore

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/claimward/claimward-vpn-app-osx/internal/helperclient"
	"github.com/claimward/claimward-vpn-app-osx/internal/hproto"
	"github.com/claimward/claimward-vpn-client/pkg/auth"
	"github.com/claimward/claimward-vpn-client/pkg/client"
	"github.com/claimward/claimward-vpn-client/pkg/protocol"
	"github.com/claimward/claimward-vpn-client/pkg/routeclient"
	"github.com/claimward/claimward-vpn-client/pkg/tokenstore"
	"github.com/claimward/claimward-vpn-client/pkg/wgkey"
)

// Core is the app's stateful service.
type Core struct {
	mu     sync.Mutex
	cfg    *Config
	api    *client.Client
	helper *helperclient.Client

	connected bool
	iface     string
	assigned  string
	// watchCancel stops the gRPC route watcher (set while connected).
	watchCancel context.CancelFunc
	// pending device-code prompt (GitHub device flow), surfaced via Status.
	devURI  string
	devCode string
	// log is a capped ring of timestamped connection-process lines (verbose).
	log []string
}

// logf appends a timestamped line to the verbose connection log (surfaced in
// the UI). Must NOT be called while holding c.mu.
func (c *Core) logf(format string, args ...any) {
	line := time.Now().Format("15:04:05") + "  " + fmt.Sprintf(format, args...)
	c.mu.Lock()
	c.log = append(c.log, line)
	if len(c.log) > 200 {
		c.log = c.log[len(c.log)-200:]
	}
	c.mu.Unlock()
}

// New builds a Core from config.
func New(cfg *Config) *Core {
	return &Core{
		cfg:    cfg,
		api:    client.New(cfg.ServerURL),
		helper: helperclient.New(cfg.SocketPath),
	}
}

// deps returns a consistent snapshot of the config and clients under the lock,
// so a concurrent UpdateConfig can't tear them.
func (c *Core) deps() (Config, *client.Client, *helperclient.Client) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return *c.cfg, c.api, c.helper
}

// Config returns the current configuration.
func (c *Core) Config() Config {
	c.mu.Lock()
	defer c.mu.Unlock()
	return *c.cfg
}

// UpdateConfig persists a new configuration and applies it live.
func (c *Core) UpdateConfig(in Config) error {
	if in.Provider == "" {
		in.Provider = "github"
	}
	if err := SaveConfig(&in); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cfg = &in
	c.api = client.New(in.ServerURL)
	c.helper = helperclient.New(in.SocketPath)
	return nil
}

// Status is a snapshot for the UI and tray.
type Status struct {
	ConfigOK        bool   `json:"config_ok"`
	ConfigError     string `json:"config_error,omitempty"`
	Provider        string `json:"provider,omitempty"`
	LoggedIn        bool   `json:"logged_in"`
	Email           string `json:"email,omitempty"`
	HelperInstalled bool   `json:"helper_installed"`
	Connected       bool   `json:"connected"`
	Interface       string `json:"interface,omitempty"`
	AssignedIP      string `json:"assigned_ip,omitempty"`
	ServerURL       string `json:"server_url,omitempty"`
	// Device-code prompt, set while a GitHub device-flow login is in progress.
	DeviceVerificationURI string `json:"device_verification_uri,omitempty"`
	DeviceUserCode        string `json:"device_user_code,omitempty"`
	// Log is the verbose connection-process log (most recent lines).
	Log []string `json:"log,omitempty"`
}

// Status returns the current state.
func (c *Core) Status() Status {
	cfg, _, helper := c.deps()

	st := Status{ServerURL: cfg.ServerURL, Provider: cfg.Provider}
	if err := cfg.Validate(); err != nil {
		st.ConfigError = err.Error()
	} else {
		st.ConfigOK = true
	}

	if sess, _ := tokenstore.Load(); sess != nil && sess.Bearer != "" {
		st.LoggedIn = true
		if sess.BearerKind == string(auth.KindIDToken) {
			st.Email = emailFromIDToken(sess.Bearer)
		}
	}

	st.HelperInstalled = helper.Available()
	if st.HelperInstalled {
		if hresp, err := helper.Status(); err == nil {
			st.Connected = hresp.Connected
			st.Interface = hresp.Interface
		}
	}

	c.mu.Lock()
	if st.Connected {
		st.AssignedIP = c.assigned
	}
	st.DeviceVerificationURI = c.devURI
	st.DeviceUserCode = c.devCode
	if n := len(c.log); n > 0 {
		st.Log = append([]string(nil), c.log...)
	}
	c.mu.Unlock()
	return st
}

func authConfig(cfg Config) auth.Config {
	return auth.Config{
		Provider:       cfg.Provider,
		GitHubClientID: cfg.GitHubClientID,
		OIDCIssuer:     cfg.OIDCIssuer,
		OIDCClientID:   cfg.OIDCClientID,
	}
}

// Login runs the interactive provider flow and persists the session. For the
// GitHub device flow, the verification URL + user code are exposed via Status
// while the user completes sign-in in their browser.
func (c *Core) Login(ctx context.Context) error {
	cfg, _, _ := c.deps()
	if err := cfg.Validate(); err != nil {
		return err
	}
	provider, err := auth.New(authConfig(cfg))
	if err != nil {
		return err
	}

	onPrompt := func(p auth.DevicePrompt) {
		c.mu.Lock()
		c.devURI, c.devCode = p.VerificationURI, p.UserCode
		c.mu.Unlock()
	}
	defer func() {
		c.mu.Lock()
		c.devURI, c.devCode = "", ""
		c.mu.Unlock()
	}()

	c.logf("sign-in via %s…", provider.Name())
	tok, err := provider.Login(ctx, onPrompt)
	if err != nil {
		c.logf("sign-in FAILED: %v", err)
		return err
	}
	c.logf("signed in")

	sess, _ := tokenstore.Load()
	if sess == nil {
		sess = &tokenstore.Session{}
	}
	sess.Provider = provider.Name()
	sess.Bearer = tok.Value
	sess.BearerKind = string(tok.Kind)
	sess.RefreshToken = tok.Refresh
	sess.Expiry = tok.Expiry
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
	cfg, api, helper := c.deps()
	if err := cfg.Validate(); err != nil {
		return err
	}
	sess, err := tokenstore.Load()
	if err != nil {
		return err
	}
	if sess == nil || sess.Bearer == "" {
		return fmt.Errorf("not signed in")
	}
	pair, err := wgkey.ParsePrivate(sess.WGPrivateKey)
	if err != nil {
		return fmt.Errorf("device key invalid, sign in again: %w", err)
	}

	host, _ := os.Hostname()
	c.logf("enroll: POST %s/api/v1/enroll", cfg.ServerURL)
	resp, err := api.Enroll(ctx, sess.Bearer, pair.Public, protocol.DeviceInfo{
		Name: host, OS: "darwin", Platform: "app-osx",
	})
	if err != nil {
		c.logf("enroll FAILED: %v", err)
		return fmt.Errorf("enroll: %w", err)
	}
	c.logf("enrolled: ip=%s endpoint=%s routes=%v", resp.AssignedIP, resp.Endpoint, resp.AllowedIPs)

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
	c.logf("bringing up tunnel via helper…")
	hresp, err := helper.Up(spec)
	if err != nil {
		c.logf("tunnel up FAILED: %v", err)
		return err
	}

	c.mu.Lock()
	c.connected = true
	c.iface = hresp.Interface
	c.assigned = resp.AssignedIP
	c.mu.Unlock()
	c.logf("connected: interface=%s ip=%s", hresp.Interface, resp.AssignedIP)

	// Watch the server for live route updates and apply them via the helper.
	if resp.GRPCEndpoint != "" {
		c.logf("watching routes at %s", resp.GRPCEndpoint)
		c.startRouteWatch(resp.GRPCEndpoint, sess.Bearer, pair.Public.String(), helper)
	}
	return nil
}

func (c *Core) startRouteWatch(endpoint, bearer, pubKey string, helper *helperclient.Client) {
	c.mu.Lock()
	if c.watchCancel != nil {
		c.watchCancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	c.watchCancel = cancel
	c.mu.Unlock()
	go func() {
		err := routeclient.Watch(ctx, endpoint, bearer, pubKey, func(u routeclient.Update) {
			c.logf("route update (serial %d): %v", u.Serial, u.AllowedIPs)
			if _, herr := helper.UpdateRoutes(u.AllowedIPs); herr != nil {
				c.logf("apply routes FAILED: %v", herr)
			}
		})
		if err != nil && ctx.Err() == nil {
			c.logf("route watch ended: %v", err)
		}
	}()
}

func (c *Core) stopRouteWatch() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.watchCancel != nil {
		c.watchCancel()
		c.watchCancel = nil
	}
}

// Disconnect tears the tunnel down and deregisters the peer.
func (c *Core) Disconnect(ctx context.Context) error {
	c.logf("disconnecting…")
	c.stopRouteWatch()
	_, api, helper := c.deps()
	_, derr := helper.Down()

	if sess, _ := tokenstore.Load(); sess != nil && sess.WGPrivateKey != "" {
		if pair, err := wgkey.ParsePrivate(sess.WGPrivateKey); err == nil {
			_ = api.Deregister(ctx, sess.Bearer, pair.Public)
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
