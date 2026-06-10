// Package helperclient is the app-side client for the privileged helper.
package helperclient

import (
	"encoding/json"
	"fmt"
	"net"
	"time"

	"github.com/claimward/claimward-vpn-app-osx/internal/hproto"
)

// Client talks to the helper over its Unix socket.
type Client struct {
	SocketPath string
	Timeout    time.Duration
}

// New returns a Client for the given socket path (empty = default).
func New(socketPath string) *Client {
	if socketPath == "" {
		socketPath = hproto.DefaultSocketPath
	}
	return &Client{SocketPath: socketPath, Timeout: 30 * time.Second}
}

// Up brings the tunnel up with the given spec.
func (c *Client) Up(spec hproto.TunnelSpec) (*hproto.Response, error) {
	return c.call(hproto.Request{Action: hproto.ActionUp, Tunnel: &spec})
}

// Down tears the tunnel down.
func (c *Client) Down() (*hproto.Response, error) {
	return c.call(hproto.Request{Action: hproto.ActionDown})
}

// Status queries the helper's current state.
func (c *Client) Status() (*hproto.Response, error) {
	return c.call(hproto.Request{Action: hproto.ActionStatus})
}

// Available reports whether the helper socket is reachable.
func (c *Client) Available() bool {
	conn, err := net.DialTimeout("unix", c.SocketPath, 2*time.Second)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func (c *Client) call(req hproto.Request) (*hproto.Response, error) {
	conn, err := net.DialTimeout("unix", c.SocketPath, c.Timeout)
	if err != nil {
		return nil, fmt.Errorf("helper not reachable at %s (is it installed and running?): %w", c.SocketPath, err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(c.Timeout))

	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	var resp hproto.Response
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if !resp.OK && resp.Error != "" {
		return &resp, fmt.Errorf("helper: %s", resp.Error)
	}
	return &resp, nil
}
