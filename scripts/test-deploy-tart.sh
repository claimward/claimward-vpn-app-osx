#!/usr/bin/env bash
#
# Deployment smoke test in a throwaway macOS VM (Tart, Apple Silicon).
#
# Clones a cirruslabs macOS base image, boots it headless, copies the built
# Claimward.dmg in, installs the app to /Applications and the privileged helper
# as a LaunchDaemon, then verifies the install (codesign, Gatekeeper, daemon,
# helper socket). The VM is deleted on exit.
#
# Connectivity: the cirruslabs *base* image has no Tart Guest Agent (so `tart
# exec` is unavailable) and only allows SSH password auth. sshpass isn't in the
# pkgx pantry, so we feed the password through OpenSSH's SSH_ASKPASS mechanism
# (SSH_ASKPASS_REQUIRE=force) — which, unlike expect, works without a TTY (e.g.
# under CI / headless background runners).
#
# The GUI tray/webview is NOT exercised (no display in a headless VM); this
# validates the *deployment* path.
set -euo pipefail

VM="${VM:-claimward-deploy-test}"
IMAGE="${IMAGE:-ghcr.io/cirruslabs/macos-sequoia-base:latest}"
VMUSER="${VMUSER:-admin}"
VMPASS="${VMPASS:-admin}"

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DMG="$ROOT/dist/Claimward.dmg"
PLIST="$ROOT/deploy/com.claimward.helper.plist"
WORK="$(mktemp -d)"
IP=""

# Non-interactive password auth via SSH_ASKPASS (no TTY/expect needed).
printf '#!/bin/sh\necho %s\n' "$VMPASS" > "$WORK/askpass.sh"
chmod +x "$WORK/askpass.sh"
export SSH_ASKPASS="$WORK/askpass.sh" SSH_ASKPASS_REQUIRE=force DISPLAY=:0
SSHOPTS=(-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null
         -o LogLevel=ERROR -o ConnectTimeout=10 -o NumberOfPasswordPrompts=1)

log()    { printf '\n\033[1;36m==> %s\033[0m\n' "$*"; }
vm_ssh() { ssh "${SSHOPTS[@]}" "$VMUSER@$IP" "$@" 2>&1 | grep -vE "X11|Warning: Permanently" || true; }
vm_scp() { scp "${SSHOPTS[@]}" "$1" "$VMUSER@$IP:$2" 2>&1 | grep -vE "X11|Warning: Permanently" || true; }

cleanup() {
  log "cleanup: deleting VM $VM"
  tart stop "$VM" >/dev/null 2>&1 || true
  tart delete "$VM" >/dev/null 2>&1 || true
  rm -rf "$WORK"
}
trap cleanup EXIT

[ -f "$DMG" ]   || { echo "missing $DMG — run 'task dmg' first"; exit 1; }
[ -f "$PLIST" ] || { echo "missing $PLIST"; exit 1; }

cat > "$WORK/install.sh" <<'REMOTE_EOF'
#!/bin/bash
set -e
hdiutil attach /tmp/Claimward.dmg -nobrowse -mountpoint /Volumes/Claimward >/dev/null
rm -rf /Applications/Claimward.app
cp -R /Volumes/Claimward/Claimward.app /Applications/
hdiutil detach /Volumes/Claimward >/dev/null
mkdir -p /Library/PrivilegedHelperTools
cp /Applications/Claimward.app/Contents/MacOS/claimward-helper /Library/PrivilegedHelperTools/claimward-helper
cp /tmp/com.claimward.helper.plist /Library/LaunchDaemons/com.claimward.helper.plist
launchctl bootout system /Library/LaunchDaemons/com.claimward.helper.plist 2>/dev/null || true
launchctl bootstrap system /Library/LaunchDaemons/com.claimward.helper.plist
launchctl enable system/com.claimward.helper || true
sleep 3
echo "========== RESULTS =========="
echo "[app]        $([ -d /Applications/Claimward.app ] && echo INSTALLED || echo MISSING)"
printf "[codesign]   "; codesign -dv /Applications/Claimward.app 2>&1 | grep -E "Identifier=|Signature=" | tr '\n' ' '; echo
printf "[gatekeeper] "; spctl -a -t exec -vv /Applications/Claimward.app 2>&1 | head -1
printf "[daemon]     "; launchctl print system/com.claimward.helper 2>/dev/null | grep -m1 "state = " | sed 's/^[[:space:]]*//' || echo "not loaded"
echo "[socket]     $([ -S /var/run/claimward-helper.sock ] && echo PRESENT || echo ABSENT) (/var/run/claimward-helper.sock)"
echo "============================="
REMOTE_EOF

log "clone $IMAGE -> $VM"
tart delete "$VM" >/dev/null 2>&1 || true
tart clone "$IMAGE" "$VM"

log "boot VM (headless)"
tart run "$VM" --no-graphics >"$WORK/tart.log" 2>&1 &

log "wait for IP"
IP="$(tart ip "$VM" --wait 180)"
echo "VM IP: $IP"

log "wait for SSH port 22"
for _ in $(seq 1 90); do
  if nc -z -G 3 "$IP" 22 >/dev/null 2>&1; then echo "port 22 open"; break; fi
  sleep 2
done
sleep 5

log "copy DMG + helper plist + install script into the VM"
vm_scp "$DMG" /tmp/Claimward.dmg
vm_scp "$PLIST" /tmp/com.claimward.helper.plist
vm_scp "$WORK/install.sh" /tmp/install.sh

log "run install + verification (sudo) inside the VM"
vm_ssh "sudo bash /tmp/install.sh"

log "deployment test done"
