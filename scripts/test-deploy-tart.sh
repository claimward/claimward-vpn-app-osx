#!/usr/bin/env bash
#
# Deployment smoke test in a throwaway macOS VM (Tart, Apple Silicon).
#
# Clones a cirruslabs macOS base image, boots it headless, copies the built
# Claimward.dmg in, installs the app to /Applications and the privileged helper
# as a LaunchDaemon, then verifies the install (codesign, Gatekeeper, daemon,
# helper socket). The VM is deleted on exit.
#
# Connectivity: the cirruslabs *base* image has no Tart Guest Agent, so `tart
# exec` is unavailable — we use SSH. sshpass isn't in the pkgx pantry, so the
# password is driven with expect (built into macOS). We wait for port 22 with nc
# (no auth, no agent throttling) and keep auths few and sequential.
#
# The GUI tray/webview is NOT exercised (no display in a headless VM); this
# validates the *deployment* path.
set -euo pipefail

VM="${VM:-claimward-deploy-test}"
IMAGE="${IMAGE:-ghcr.io/cirruslabs/macos-sequoia-base:latest}"
VMUSER="${VMUSER:-admin}"
VMPASS="${VMPASS:-admin}"
SSHOPTS="-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR -o ConnectTimeout=10"

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DMG="$ROOT/dist/Claimward.dmg"
PLIST="$ROOT/deploy/com.claimward.helper.plist"
WORK="$(mktemp -d)"
IP=""

log() { printf '\n\033[1;36m==> %s\033[0m\n' "$*"; }

cleanup() {
  log "cleanup: deleting VM $VM"
  tart stop "$VM" >/dev/null 2>&1 || true
  tart delete "$VM" >/dev/null 2>&1 || true
  rm -rf "$WORK"
}
trap cleanup EXIT

# Run a SIMPLE remote command (no pipes/quotes — Tcl word-splits the spawn line).
vm_ssh() {
  expect <<EXP
set timeout 180
spawn ssh $SSHOPTS $VMUSER@$IP $1
expect { -re "(P|p)assword:" { send "$VMPASS\r"; exp_continue } eof }
catch wait result
exit [lindex \$result 3]
EXP
}

# Copy a local file into the VM (paths must contain no spaces). The remote path
# is intentionally unquoted: under expect/Tcl, quotes mid-word reach scp literally.
vm_scp() {
  expect <<EXP
set timeout 600
spawn scp $SSHOPTS $1 $VMUSER@$IP:$2
expect { -re "(P|p)assword:" { send "$VMPASS\r"; exp_continue } eof }
catch wait result
exit [lindex \$result 3]
EXP
}

[ -f "$DMG" ]   || { echo "missing $DMG — run 'task dmg' first"; exit 1; }
[ -f "$PLIST" ] || { echo "missing $PLIST"; exit 1; }

cat > "$WORK/install.sh" <<'REMOTE_EOF'
#!/bin/bash
set -e
echo "### mount DMG"
hdiutil attach /tmp/Claimward.dmg -nobrowse -mountpoint /Volumes/Claimward >/dev/null
echo "### copy app to /Applications"
rm -rf /Applications/Claimward.app
cp -R /Volumes/Claimward/Claimward.app /Applications/
hdiutil detach /Volumes/Claimward >/dev/null
echo "### install privileged helper (LaunchDaemon)"
mkdir -p /Library/PrivilegedHelperTools
cp /Applications/Claimward.app/Contents/MacOS/claimward-helper /Library/PrivilegedHelperTools/claimward-helper
cp /tmp/com.claimward.helper.plist /Library/LaunchDaemons/com.claimward.helper.plist
launchctl bootout system /Library/LaunchDaemons/com.claimward.helper.plist 2>/dev/null || true
launchctl bootstrap system /Library/LaunchDaemons/com.claimward.helper.plist
launchctl enable system/com.claimward.helper || true
sleep 3
echo
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
sleep 30

log "copy DMG + helper plist + install script into the VM"
vm_scp "$DMG" /tmp/Claimward.dmg
vm_scp "$PLIST" /tmp/com.claimward.helper.plist
vm_scp "$WORK/install.sh" /tmp/install.sh

log "run install + verification (sudo) inside the VM"
vm_ssh "sudo bash /tmp/install.sh"

log "deployment test PASSED"
