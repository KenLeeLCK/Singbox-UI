#!/usr/bin/env bash
set -euo pipefail
[[ $(uname -s) == Linux && $EUID == 0 ]] || { echo 'Run as root on Linux.' >&2; exit 1; }
exec 9>/run/sing-box-manager-install.lock
flock -n 9 || { echo 'Install/update running.' >&2; exit 1; }
systemctl stop sing-box-manager.service || true
if [[ -f /var/lib/sing-box-manager-helper/transaction.json ]]; then
  echo "An unconfirmed sing-box deployment exists. Helper retained for recovery; resolve it before uninstalling. Web service is stopped." >&2
  exit 1
fi
systemctl disable --now sing-box-manager.service sing-box-manager-helper.service || true
rm -f /etc/systemd/system/sing-box-manager.service /etc/systemd/system/sing-box-manager-helper.service
systemctl daemon-reload
rm -rf /usr/local/lib/sing-box-manager
printf '%s\n' 'Manager removed. sing-box, its service/configuration, Manager history/state, service account, and Tailscale routes are preserved.' 'Remove any dedicated Tailscale route manually after inspecting tailscale serve status.'
