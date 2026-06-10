# claimward-vpn-app-osx

The Claimward VPN client for macOS: a **menu-bar (tray) app written in Go**, whose
**entire user interface is a Svelte single-page app rendered in a webview**.

## How it's put together

```
┌────────────────────────── claimward-app (tray process) ──────────────────────────┐
│  fyne.io/systray   ── menu: status / Connect / Disconnect / Open / Quit           │
│  uiserver          ── loopback HTTP: serves embedded Svelte SPA + JSON API        │
│  appcore           ── OIDC login, enroll (claimward-vpn-client), drive the helper │
└───────────────┬──────────────────────────────────────────────┬───────────────────┘
                │ spawns "claimward-app ui <url>"                │ Unix socket (JSON)
                ▼                                                ▼
   ┌─────────────────────────┐                    ┌──────────────────────────────┐
   │ webview (WKWebView)      │  fetch /api/* ──►  │ claimward-helper (root daemon)│
   │ renders the Svelte UI    │                    │ wireguard-go: utun up/down    │
   └─────────────────────────┘                    └──────────────────────────────┘
```

- **Tray process** owns all state. It serves the UI and a token-guarded JSON API
  on `127.0.0.1`, and talks to the helper.
- **Webview** is a thin chromeless window pointed at the loopback URL — the UI is
  100% Svelte (`frontend/`), built with Vite and embedded via `go:embed`.
- **Privileged helper** is the only component that runs as root. It creates the
  `utun` device and brings the WireGuard tunnel up/down via `wireguard-go`
  (`claimward-vpn-client/pkg/wgtun`). The app sends it a tunnel spec over a Unix
  socket.

Why a separate helper + a separate webview process? On macOS only one Cocoa run
loop can own the main thread, so the tray and the webview live in different
processes; and tunnel setup needs root, which the unprivileged app must not have.

## Layout

| Path | What |
|------|------|
| `cmd/claimward-app` | tray process (+ `ui` subcommand = webview window) |
| `cmd/claimward-helper` | privileged root daemon (LaunchDaemon) |
| `internal/appcore` | login / enroll / connect logic + config |
| `internal/uiserver` | embedded Svelte SPA + loopback JSON API |
| `internal/helperclient` | app→helper socket client |
| `internal/hproto` | helper wire protocol |
| `frontend/` | Svelte + Vite UI (builds to `internal/uiserver/dist`) |
| `deploy/`, `scripts/` | LaunchDaemon plist + install/uninstall |

## Build

```sh
# 1. Build the Svelte UI (embedded into the Go binary)
cd frontend && npm install && npm run build && cd ..

# 2. Build the app and helper (cgo: WebKit + Cocoa)
CGO_ENABLED=1 go build -o bin/claimward-app    ./cmd/claimward-app
CGO_ENABLED=1 go build -o bin/claimward-helper  ./cmd/claimward-helper
```

## Configure

Create `~/Library/Application Support/Claimward/config.json` (or use the
`CLAIMWARD_*` env vars). **GitHub is the default provider** (OAuth device flow):

```json
{
  "server_url": "https://vpn.example.com",
  "provider": "github",
  "github_client_id": "Iv1.0123456789abcdef"
}
```

To use an OIDC provider instead:

```json
{
  "server_url": "https://vpn.example.com",
  "provider": "oidc",
  "oidc_issuer": "https://accounts.google.com",
  "oidc_client_id": "xxxx.apps.googleusercontent.com"
}
```

## Install the helper, then run

```sh
sudo ./scripts/install-helper.sh   # root LaunchDaemon + Unix socket
./bin/claimward-app                # tray app; click Connect
```

## MVP notes / hardening TODO

- The helper socket is `0666` for the MVP. Before shipping: dedicated group +
  `0660`, and verify the peer's credentials (and ideally code-sign + SMJobBless).
- Session tokens live in a `0600` file (via `claimward-vpn-client/pkg/tokenstore`);
  graduate to the macOS Keychain.
- DNS push and split-tunnel polish are TODO (see `pkg/wgtun`).
- App is not yet bundled as a signed `.app`/notarized; that's packaging work.

## License

BSD 3-Clause — see [LICENSE](LICENSE).
