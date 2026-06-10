package appcore

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// Config is the app's runtime configuration. In a shipped build these values
// are typically baked in; for development they come from a JSON file under
// "~/Library/Application Support/Claimward/config.json" or environment overrides.
type Config struct {
	ServerURL string `json:"server_url"` // claimward-vpn-server base URL

	Provider       string `json:"provider"`         // "github" (default) | "oidc"
	GitHubClientID string `json:"github_client_id"` // GitHub OAuth app client id (device flow)

	OIDCIssuer   string `json:"oidc_issuer"`    // OIDC issuer (discovery)
	OIDCClientID string `json:"oidc_client_id"` // OIDC client id

	SocketPath string `json:"socket_path"` // helper socket (empty = default)
}

// LoadConfig reads config.json (if present) then applies environment overrides.
func LoadConfig() (*Config, error) {
	c := &Config{}
	if p, err := configPath(); err == nil {
		if data, rerr := os.ReadFile(p); rerr == nil {
			if jerr := json.Unmarshal(data, c); jerr != nil {
				return nil, jerr
			}
		} else if !errors.Is(rerr, fs.ErrNotExist) {
			return nil, rerr
		}
	}
	override(&c.ServerURL, "CLAIMWARD_SERVER")
	override(&c.Provider, "CLAIMWARD_AUTH_PROVIDER")
	override(&c.GitHubClientID, "CLAIMWARD_GITHUB_CLIENT_ID")
	override(&c.OIDCIssuer, "CLAIMWARD_OIDC_ISSUER")
	override(&c.OIDCClientID, "CLAIMWARD_OIDC_CLIENT_ID")
	override(&c.SocketPath, "CLAIMWARD_HELPER_SOCKET")
	if c.Provider == "" {
		c.Provider = "github"
	}
	return c, nil
}

// Validate reports whether the minimum required fields are present.
func (c *Config) Validate() error {
	var missing []string
	if c.ServerURL == "" {
		missing = append(missing, "server_url")
	}
	switch c.Provider {
	case "github":
		if c.GitHubClientID == "" {
			missing = append(missing, "github_client_id")
		}
	case "oidc":
		if c.OIDCIssuer == "" {
			missing = append(missing, "oidc_issuer")
		}
		if c.OIDCClientID == "" {
			missing = append(missing, "oidc_client_id")
		}
	default:
		return errors.New("invalid provider: " + c.Provider + ` (want "github" or "oidc")`)
	}
	if len(missing) > 0 {
		return errors.New("missing config: " + join(missing))
	}
	return nil
}

func configPath() (string, error) {
	dir, err := os.UserConfigDir() // ~/Library/Application Support on macOS
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "Claimward", "config.json"), nil
}

func override(field *string, env string) {
	if v := os.Getenv(env); v != "" {
		*field = v
	}
}

func join(s []string) string {
	out := ""
	for i, v := range s {
		if i > 0 {
			out += ", "
		}
		out += v
	}
	return out
}
