// Package hproto defines the line protocol spoken between the unprivileged
// Claimward app and the privileged helper over a Unix domain socket.
//
// One JSON request per connection, one JSON response back. Keeping it tiny and
// self-contained means the helper has a minimal attack surface.
package hproto

// DefaultSocketPath is where the privileged helper listens.
const DefaultSocketPath = "/var/run/claimward-helper.sock"

// Action values.
const (
	ActionUp     = "up"
	ActionDown   = "down"
	ActionStatus = "status"
)

// Request is sent by the app to the helper.
type Request struct {
	Action string      `json:"action"`
	Tunnel *TunnelSpec `json:"tunnel,omitempty"` // required for ActionUp
}

// TunnelSpec is a JSON-friendly mirror of wgtun.Config (keys as base64 strings).
type TunnelSpec struct {
	PrivateKey      string   `json:"private_key"`       // base64
	ServerPublicKey string   `json:"server_public_key"` // base64
	Endpoint        string   `json:"endpoint"`          // host:port
	Address         string   `json:"address"`           // CIDR, e.g. 10.80.0.5/32
	AllowedIPs      []string `json:"allowed_ips"`
	DNS             []string `json:"dns,omitempty"`
	MTU             int      `json:"mtu,omitempty"`
	Keepalive       int      `json:"keepalive,omitempty"`
}

// Response is returned by the helper.
type Response struct {
	OK        bool   `json:"ok"`
	Error     string `json:"error,omitempty"`
	Connected bool   `json:"connected"`
	Interface string `json:"interface,omitempty"`
}
