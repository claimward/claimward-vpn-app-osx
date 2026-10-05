#!/usr/bin/env bash
#
# Installs the Claimward privileged helper as a root LaunchDaemon.
# Must be run with sudo. Builds the helper if a prebuilt binary isn't supplied.
#
#   sudo ./scripts/install-helper.sh https://vpn.example.org [more servers...]
#
# The servers are the only claimward-vpn-server URLs the helper will enroll
# with: it runs as root, and anything that can reach its socket could
# otherwise point it at a server of its own (pkg/helper). Its socket is for
# root and the admin group.
#
set -euo pipefail

LABEL="com.claimward.helper"
HELPER_DST="/Library/PrivilegedHelperTools/claimward-helper"
PLIST_DST="/Library/LaunchDaemons/${LABEL}.plist"

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PLIST_SRC="${ROOT}/deploy/${LABEL}.plist"

if [[ $EUID -ne 0 ]]; then
  echo "error: run with sudo" >&2
  exit 1
fi
if [[ $# -lt 1 ]]; then
  echo "usage: sudo $0 https://vpn.example.org [more servers...]" >&2
  exit 1
fi

# Build the helper if not already built.
HELPER_BIN="${ROOT}/bin/claimward-helper"
if [[ ! -x "$HELPER_BIN" ]]; then
  echo "Building helper…"
  ( cd "$ROOT" && CGO_ENABLED=0 go build -o bin/claimward-helper ./cmd/claimward-helper )
fi

echo "Installing helper -> ${HELPER_DST}"
mkdir -p /Library/PrivilegedHelperTools
install -m 0755 -o root -g wheel "$HELPER_BIN" "$HELPER_DST"

CONF_DIR="/Library/Application Support/Claimward"
CONF="${CONF_DIR}/helper.json"
echo "Writing ${CONF} (servers: $*)"
mkdir -p "$CONF_DIR"
chown root:wheel "$CONF_DIR"
chmod 0755 "$CONF_DIR"
{
  printf '{"servers": ['
  sep=""
  for s in "$@"; do printf '%s"%s"' "$sep" "$s"; sep=", "; done
  printf ']}\n'
} > "${CONF}.tmp"
install -m 0644 -o root -g wheel "${CONF}.tmp" "$CONF"
rm -f "${CONF}.tmp"

echo "Installing LaunchDaemon -> ${PLIST_DST}"
install -m 0644 -o root -g wheel "$PLIST_SRC" "$PLIST_DST"

echo "Loading daemon…"
launchctl bootout system "$PLIST_DST" 2>/dev/null || true
launchctl bootstrap system "$PLIST_DST"
launchctl enable "system/${LABEL}"

echo "Done. Helper socket: /var/run/claimward-helper.sock (root:admin, 0660)"
echo "Logs: /var/log/claimward-helper.log"
