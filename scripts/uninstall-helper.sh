#!/usr/bin/env bash
#
# Removes the Claimward privileged helper LaunchDaemon. Run with sudo.
set -euo pipefail

LABEL="com.claimward.helper"
PLIST_DST="/Library/LaunchDaemons/${LABEL}.plist"

if [[ $EUID -ne 0 ]]; then
  echo "error: run with sudo" >&2
  exit 1
fi

launchctl bootout system "$PLIST_DST" 2>/dev/null || true
rm -f "$PLIST_DST"
rm -f /Library/PrivilegedHelperTools/claimward-helper
rm -f /var/run/claimward-helper.sock
echo "Uninstalled ${LABEL}."
