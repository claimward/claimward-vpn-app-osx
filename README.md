# claimward-vpn-app-osx


The Claimward VPN client for macOS: a **menu-bar (tray) app written in Go**, whose
**entire user interface is a Svelte single-page app rendered in a webview**.

A signed DMG of each tagged version is attached to its
[release](https://github.com/claimward/claimward-vpn-app-osx/releases) (`Claimward.dmg`, from
v0.1.0); it still needs the helper installed, see below.

## How it's put together

```
┌────────────────────────── claimward-app (tray process) ──────────────────────────┐
│  fyne.io/systray  ── menu: status, Open, Configuration, Connect, Disconnect, Quit│
│  uiserver         ── loopback HTTP: serves embedded Svelte SPA + JSON API        │
│  appcore          ── sign-in, tenant choice, helper client (claimward-vpn-client)│
└───────────────┬──────────────────────────────────────────────┬───────────────────┘
                │ spawns "claimward-app ui <url>"                │ Unix socket (JSON)
                ▼                                                ▼
   ┌─────────────────────────┐                    ┌──────────────────────────────┐
   │ webview (WKWebView)      │  fetch /api/* ──►  │ claimward-helper (root daemon)│
   │ renders the Svelte UI    │                    │ enrolls, wireguard-go utun    │
   └─────────────────────────┘                    └──────────────────────────────┘
```

- **Tray process** owns all state. It serves the UI and a token-guarded JSON API
  on `127.0.0.1`, and talks to the helper.
- **Webview** is a thin chromeless window pointed at the loopback URL — the UI is
  100% Svelte (`frontend/`), built with Vite and embedded via `go:embed`.
- **Privileged helper** is the only component that runs as root. The app sends
  it a *connect* request over a Unix socket (server URL, bearer, device key,
  tenant); the helper enrolls with that server — only if `helper.json` names it —
  creates the `utun` device and brings the WireGuard tunnel up/down via
  `wireguard-go` (`claimward-vpn-client/pkg/helper`, `pkg/wgtun`), and watches
  the route pushes.

Why a separate helper + a separate webview process? On macOS only one Cocoa run
loop can own the main thread, so the tray and the webview live in different
processes; and tunnel setup needs root, which the unprivileged app must not have.

## Layout

| Path | What |
|------|------|
| `cmd/claimward-app` | tray process (+ `ui` subcommand = webview window) |
| `cmd/claimward-helper` | privileged root daemon (LaunchDaemon) |
| `internal/uiserver` | embedded Svelte SPA + loopback JSON API |
| `internal/brand` | the brand assets (tray icon) |
| `frontend/` | Svelte + Vite UI (builds to `internal/uiserver/dist`) |
| `deploy/`, `scripts/` | LaunchDaemon plist + install/uninstall |

The app's logic (sign-in, tenant choice, connect) and the helper's
(`pkg/appcore`, `pkg/helper`, `pkg/hproto`, `pkg/helperclient`) are in
[claimward-vpn-client](https://github.com/claimward/claimward-vpn-client), shared
with the Linux and Windows apps. This repository is the macOS shell around
them.

## Quick start (Task)

With [go-task](https://taskfile.dev) (`pkgx install task`):

```sh
task config:init     # write a starter ~/Library/Application Support/Claimward/config.json
task install-helper SERVER=https://vpn.example.org  # build + install the root helper (asks for sudo)
task start:bundle    # build Claimward.app and launch it (recommended)
```

`task --list` shows everything (`ui`, `build`, `bundle`, `run`, `dev:ui`, …).

> **Run the bundle, not the bare binary.** Use **`task start:bundle`**. On
> modern macOS a bare binary launched from a menu-bar agent can't bring its
> window to the foreground, so the dashboard opens *behind* other windows. The
> `.app` bundle (a menu-bar agent embedding a regular **Dashboard.app**) fixes
> this — "Open Claimward" then activates the window properly. `task start` (bare
> binary) is for quick dev only.

## Build (manual)

```sh
# 1. Build the Svelte UI (embedded into the Go binary)
cd frontend && npm install && npm run build && cd ..

# 2. Build the app (cgo: WebKit + Cocoa) and the helper (pure Go)
CGO_ENABLED=1 go build -o bin/claimward-app    ./cmd/claimward-app
CGO_ENABLED=0 go build -o bin/claimward-helper  ./cmd/claimward-helper
```

## Configure

Create `~/Library/Application Support/Claimward/config.json` (or use the
`CLAIMWARD_SERVER`, `CLAIMWARD_AUTH_PROVIDER`, `CLAIMWARD_GITHUB_CLIENT_ID`,
`CLAIMWARD_OIDC_ISSUER`, `CLAIMWARD_OIDC_CLIENT_ID`, `CLAIMWARD_HELPER_SOCKET`).
The menu's "Configuration…" edits the same file. `server_url` must be one of
the servers the helper was installed with. **GitHub is the default provider** (OAuth device flow):

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

Or a [go-authn](https://github.com/go-authn/bridge) provider, an OpenID Connect
provider in front of a SAML federation such as RENATER, which also keeps whose
each WireGuard key is (the server runs with `AUTH_PROVIDER=go-authn`):

```json
{
  "server_url": "https://vpn.example.com",
  "provider": "go-authn",
  "oidc_issuer": "https://login.example.org",
  "oidc_client_id": "claimward"
}
```

Sign-in is the provider's device flow. At every connection the app registers
the device's **public** key there (the private key stays in the session store),
and enrolls with the token that registration returns.

## Install the helper, then run

```sh
sudo ./scripts/install-helper.sh https://vpn.example.org   # root LaunchDaemon + Unix socket; several servers may follow
./bin/claimward-app                # tray app; click Connect
```

## Tenants

A person may belong to several tenants (claimward-vpn-server matches them on
email domain, groups and institution). The window offers the tenants once the
server says there is a choice, or on request ("Choose a tenant…"), and the
choice holds for the session; a new sign-in forgets it. A Connect from the
menu bar that the server refuses for want of a choice opens the window.

## Security

- **The helper enrolls only with the servers named at install**
  (`/Library/Application Support/Claimward/helper.json`, root's, writable by
  root alone). It runs as root, and anything that can reach its socket could
  otherwise point it at a server of its own, answering with routes for every
  packet of the machine. It takes no tunnel configuration from a request.
- **Its socket, `/var/run/claimward-helper.sock`, is `0660`, `root:admin`.**
  Before v0.1.0 it was `0666`, so any local process could drive it.
- The loopback UI API is guarded by a per-launch token, compared in constant
  time.
- A route watch carries the bearer token, and runs over TLS (claimward-vpn-client).
- Session tokens and the device key live in a `0600` file (`pkg/tokenstore`,
  under `~/Library/Application Support`). Not done yet: the Keychain, installing
  the helper with SMJobBless (it is installed by `scripts/install-helper.sh`),
  and applying the server's DNS push (`pkg/wgtun` carries it but does not set
  it on macOS).

## Release & CI

`.github/workflows/release.yml` runs on `v*` tags:

- **`dmg`** (GitHub-hosted `macos-26`): builds the DMG and, when signing secrets
  are set, signs it with a Developer ID identity and notarizes + staples it; then
  uploads `Claimward.dmg` to the release. Without secrets it falls back to an
  ad-hoc signature (fine for local/VM use, but Gatekeeper blocks a *downloaded*
  unsigned DMG).
- **`deploy-test`** (self-hosted Apple Silicon): runs the Tart VM deploy test
  against the built DMG. GitHub-hosted macOS runners can't nest virtualization,
  so this needs a self-hosted runner with Tart and the repo variable
  `RUN_TART_DEPLOY_TEST=true`.

Signing/notarization secrets (optional):

| Secret | Meaning |
|--------|---------|
| `MACOS_CERT_P12` | base64 of the Developer ID Application cert (`.p12`) |
| `MACOS_CERT_PASSWORD` | password for that `.p12` |
| `MACOS_SIGN_IDENTITY` | e.g. `Developer ID Application: Acme (TEAMID)` |
| `AC_APPLE_ID` / `AC_TEAM_ID` / `AC_PASSWORD` | Apple ID + team + app-specific password for `notarytool` |

## License

BSD 3-Clause — see [LICENSE](LICENSE).
