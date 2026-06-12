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
	ActionUp           = "up"
	ActionDown         = "down"
	ActionStatus       = "status"
	ActionUpdateRoutes = "update-routes" // apply pushed routes to the live tunnel
	// ActionConnect makes the (root) helper do the whole server-facing flow:
	// enroll, bring up the tunnel, and watch routes. macOS "Local Network"
	// privacy blocks the unprivileged app from reaching a LAN server, but the
	// root helper is exempt — so the server comms live here.
	ActionConnect = "connect"
)

// Request is sent by the app to the helper.
type Request struct {
	Action string      `json:"action"`
	Tunnel *TunnelSpec `json:"tunnel,omitempty"` // required for ActionUp
	// AllowedIPs is the new routed CIDR set for ActionUpdateRoutes.
	AllowedIPs []string `json:"allowed_ips,omitempty"`
	// Connect carries what the helper needs for ActionConnect.
	Connect *ConnectSpec `json:"connect,omitempty"`
}

// ConnectSpec is the input for ActionConnect: the helper enrolls with the server
// using these, then brings up the tunnel and watches for route pushes.
type ConnectSpec struct {
	ServerURL  string `json:"server_url"`
	Bearer     string `json:"bearer"`      // OIDC id_token / access token
	PrivateKey string `json:"private_key"` // device WireGuard private key (base64)
	DeviceName string `json:"device_name"`
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
	OK         bool   `json:"ok"`
	Error      string `json:"error,omitempty"`
	Connected  bool   `json:"connected"`
	Interface  string `json:"interface,omitempty"`
	AssignedIP string `json:"assigned_ip,omitempty"`
}
