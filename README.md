# claimward-vpn-app-osx


The Claimward VPN client for macOS: a **native app written in Go** — one window and
a menu-bar status item — drawn by [go-widgets](https://github.com/go-widgets), with
the system's own text fields and pop-up buttons embedded in it. There is no webview,
no local web server and no JavaScript.

A signed DMG of each tagged version is attached to its
[release](https://github.com/claimward/claimward-vpn-app-osx/releases) (`Claimward.dmg`, from
v0.1.0); it still needs the helper installed, see below.

## How it's put together

```
┌──────────────────────────── claimward-app (one process) ─────────────────────────┐
│  window           ── go-widgets, with NSTextField / NSPopUpButton embedded       │
│  status item      ── go-widgets/tray: status, Open, Connect, Disconnect, Quit    │
│  notifications    ── tray.Notify when the tunnel drops or the session expires    │
│  ViewModel        ── internal/ui over appvm (claimward-vpn-client), bound with   │
│                      mvvm; the same model drives the Linux and Windows apps      │
└───────────────────────────────────────────────┬──────────────────────────────────┘
                                                │ Unix socket (JSON)
                                                ▼
                                 ┌──────────────────────────────┐
                                 │ claimward-helper (root daemon)│
                                 │ enrolls, wireguard-go utun    │
                                 └──────────────────────────────┘
```

- **The app** is one unprivileged process that owns all state and talks to the
  helper. The text input (the server URL) is the system's
  `NSTextField`, with its input method and spell checking; the identity-provider
  and tenant choices are `NSPopUpButton`s. The painted widgets under them are the
  fallback a host that cannot embed controls shows. Only changed rectangles are
  re-presented, taken from the view model's changes.
- **Notifications** (tunnel dropped, session expired) are posted once per
  transition and need the app to run from the `Claimward.app` bundle; from a bare
  binary they are logged instead.
- **Privileged helper** is the only component that runs as root. The app sends
  it a *connect* request over a Unix socket (server URL, bearer, device key,
  tenant); the helper enrolls with that server — only if it is named in `helper.json` or approved —
  creates the `utun` device and brings the WireGuard tunnel up/down via
  `wireguard-go` (`claimward-vpn-client/pkg/helper`, `pkg/wgtun`), and watches
  the route pushes.

Why a separate helper? Tunnel setup needs root, which the unprivileged app must
not have.

## Layout

| Path | What |
|------|------|
| `cmd/claimward-app` | the app: window, status item, notifications |
| `cmd/claimward-helper` | privileged root daemon (LaunchDaemon) |
| `internal/ui` | the view model (over `appvm`), the bound widgets, the host |
| `internal/brand` | the brand assets (status-item icon) |
| `deploy/`, `scripts/` | LaunchDaemon plist, Info.plist + install/uninstall |

The app's logic (sign-in, tenant choice, connect) and the helper's
(`pkg/appcore`, `pkg/helper`, `pkg/hproto`, `pkg/helperclient`) are in
[claimward-vpn-client](https://github.com/claimward/claimward-vpn-client), shared
with the Linux and Windows apps. This repository is the macOS shell around
them.

## The window and the status item

Closing the window hides it: the app keeps running in the menu-bar status item
and quits only through its **Quit** item. **Open Claimward** brings the window
back (one main window, never a second).

The window opens at 800 x 480 points, the size of WireGuard's macOS manager,
and is laid out as:

- **Servers sidebar** (left): one row per server, with its logo (a neutral mark
  when the server has none), its name and its state (Connected, Signed in,
  Signed out). **+**, **−** and the gear are under the list. Several servers may
  be connected at once.
- **Detail panel** (right), in the manner of WireGuard's tunnel panel: a
  key/value table with section titles that hides every empty field except
  **Status**. **Server** (name and logo, URL, provider and signed-in identity,
  tenant, exclusive, helper approval), then the **Status** row with its icon and
  the **Connect** / **Disconnect** buttons, then **Interface** (address, lease
  expiry in relative time). The values refresh every second. **Edit** is at the
  bottom right. When the server lists several identity providers, sign-in shows
  one button per provider.
- **Add a server**: **+** opens a window with a URL field only. **Next** asks the
  server for its public discovery document and previews its name, logo,
  providers and whether it is *exclusive*; the person confirms before the server
  is added. A server that does not answer as a Claimward server is told in plain
  words. The gear (or **Edit**) opens the same window on the selected server and
  can change its identity provider.
- **Approval**: Connect on a server the helper has not approved asks macOS for
  the administrator's authorization, naming the server's host, then approves it
  and connects. A refusal is shown as such. **−** offers to revoke the approval
  with the removal.
- **Exclusive servers** (one that must be the only connection, or a connection
  that blocks the others): Connect is disabled and says why, for instance
  "Alpha is connected and exclusive".

## Quick start (Task)

With [go-task](https://taskfile.dev) (`pkgx install task`):

```sh
task config:init     # write a starter ~/Library/Application Support/Claimward/config.json
task install-helper SERVER=https://vpn.example.org  # build + install the root helper (asks for sudo)
task start:bundle    # build Claimward.app and launch it (recommended)
```

`task --list` shows everything (`build`, `bundle`, `run`, `dmg`, …).

> **Run the bundle, not the bare binary.** Use **`task start:bundle`**.
> Notifications (UNUserNotificationCenter) refuse a process that is not in an
> app bundle, so the tunnel dropping would only reach the log. `task run` (bare
> binary) is for quick dev only.

## Build (manual)

```sh
# Build the app (cgo: Cocoa) and the helper (pure Go); no Node toolchain
CGO_ENABLED=1 go build -o bin/claimward-app    ./cmd/claimward-app
CGO_ENABLED=0 go build -o bin/claimward-helper  ./cmd/claimward-helper
```

## Configure

There is nothing to write by hand: **+** adds a server by its URL. The server
publishes everything else (its name, logo and identity providers: GitHub, OIDC,
[go-authn](https://github.com/go-authn/bridge), ...) in its discovery document,
and the app keeps the list in `~/Library/Application Support/Claimward/config.json`
(version 2, claimward-vpn-client v0.11.0). **A v0.9 file** (`server_url`,
`provider`, `github_client_id`, ...) **is migrated once, on first start**, and
its session is adopted by the matching server. `CLAIMWARD_HELPER_SOCKET` still
names the helper's socket.

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
email domain, groups and institution). The detail panel's Tenant pop-up lists
the tenants once the server has offered them; **Refresh** asks again. The
choice holds for the session; a new sign-in forgets it. A Connect from the
menu bar that the server refuses for want of a choice opens the window.

## Security

- **The helper enrolls only with the servers it trusts**: the ones named at
  install (`/Library/Application Support/Claimward/helper.json`, root's, writable
  by root alone) and the ones **approved** since. It runs as root, and anything
  that can reach its socket could otherwise point it at a server of its own,
  answering with routes for every packet of the machine. Approving asks macOS for
  an administrator's authorization (Authorization Services right
  `com.claimward.helper.approve-server`; the app's `macauth.Obtain` token buys one
  approval and the helper verifies and destroys it), and the approvals are kept in
  `/var/db/claimward/approved-servers.json` with a digest of the server's
  identity: a server that changes its identity must be approved again. It takes
  no tunnel configuration from a request.
- **Its socket, `/var/run/claimward-helper.sock`, is `0660`, `root:admin`.**
  Before v0.1.0 it was `0666`, so any local process could drive it.
- There is no local HTTP server and no handoff token any more (before v0.7.0 the
  UI was a webview talking to a loopback API guarded by a per-launch token): the
  window and the logic share one process. The settings form cannot change the
  helper socket (claimward-vpn-client v0.4.0).
- A route watch carries the bearer token, and runs over TLS (claimward-vpn-client).
- The tunnel's MTU is clamped to the path MTU to the server minus 60 (IPv4) or
  80 (IPv6) (claimward-vpn-client v0.6.0, `pkg/pathmtu`), so large packets are
  not lost on links with a smaller MTU.
- Session tokens and the device key live in the login Keychain, sealed through
  `pkg/tokenstore` (claimward-vpn-client v0.5.0); a `0600` file from an older
  version is moved into it on first start. There is no plaintext fallback: if
  the Keychain is locked or unavailable, sign-in fails and says so. Not done yet: installing the helper with SMJobBless
  (it is installed by `scripts/install-helper.sh`), and applying the server's DNS
  push (`pkg/wgtun` carries it but does not set it on macOS).

## Release & CI

`.github/workflows/ci.yml` runs gofmt, vet, build, the tests (`internal/ui` held
at 100% coverage) and the race detector on Apple silicon and Intel; the
go-widgets MVVM gate (`mvvmlint`) and the hand-drawn-UI gate (`bricolint`, with
its negative control); and a launch lane that builds `Claimward.app`, starts it
and reads the accessibility tree for the status item and the window (the native
fields on the Settings page are asserted by the `internal/ui` tests). The Node/Vite toolchain is gone from CI and from the
release build.

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
