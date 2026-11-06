#!/usr/bin/env bash
# Usage: sudo bash scripts/install.sh --repo OWNER/REPO --version vX.Y.Z [--tailscale]
# Offline/reviewed local build: sudo bash scripts/install.sh --local-package /absolute/staging
set -euo pipefail
umask 077
HERE=$(cd "$(dirname "$0")" && pwd)
REPO='' VERSION='' LOCAL='' TAILSCALE=0 UNIT=sing-box.service UNIT_EXPLICIT=0 UPDATE_ONLY=0
while (($#)); do
  case "$1" in
    --repo) REPO=${2:?}; shift 2;; --version) VERSION=${2:?}; shift 2;;
    --local-package) LOCAL=${2:?}; shift 2;; --tailscale) TAILSCALE=1; shift;;
    --unit) UNIT=${2:?}; UNIT_EXPLICIT=1; shift 2;; --update-only) UPDATE_ONLY=1; shift;;
    *) echo "Unknown argument $1" >&2; exit 2;;
  esac
done
[[ $(uname -s) == Linux && $EUID == 0 ]] || { echo 'Run as root on systemd Linux.' >&2; exit 1; }
[[ $UNIT =~ ^[a-zA-Z0-9_.@-]+\.service$ ]] || { echo 'Invalid unit name' >&2; exit 2; }
. /etc/os-release
[[ $ID == debian || $ID == ubuntu ]] || { echo 'Supported distributions: Debian and Ubuntu.' >&2; exit 1; }
for tool in python3 curl tar sha256sum systemctl flock useradd install; do command -v "$tool" >/dev/null || { echo "Missing prerequisite: $tool" >&2; exit 1; }; done
[[ -d /run/systemd/system ]] || { echo 'systemd must be running.' >&2; exit 1; }
exec 9>/run/sing-box-manager-install.lock
flock -n 9 || { echo 'Another install/update is running.' >&2; exit 1; }
case $(uname -m) in x86_64) ARCH=amd64;; aarch64|arm64) ARCH=arm64;; *) echo 'Only amd64/arm64 supported' >&2; exit 1;; esac
WORK=$(mktemp -d); trap 'rm -rf "$WORK"' EXIT
DEST=/usr/local/lib/sing-box-manager
if ((UNIT_EXPLICIT)); then
  UNIT=$(python3 "$HERE/discover-service.py" --unit "$UNIT")
else
  UNIT=$(python3 "$HERE/discover-service.py")
fi
# Recover an interrupted binary switch before doing new work.
if [[ -f $DEST/update-pending && -f $DEST/manager.previous ]]; then
  install -m 0755 "$DEST/manager.previous" "$DEST/manager.recovery"
  mv -f "$DEST/manager.recovery" "$DEST/manager"
  systemctl reset-failed sing-box-manager-helper.service sing-box-manager.service || true
  systemctl restart sing-box-manager-helper.service sing-box-manager.service
  rm "$DEST/update-pending"
fi
if [[ -n $LOCAL ]]; then
  PACKAGE=$(realpath "$LOCAL")
  [[ -x $PACKAGE/manager && -d $PACKAGE/deploy && -d $PACKAGE/scripts ]] || { echo 'Local package must contain executable manager, deploy/, scripts/.' >&2; exit 1; }
else
  [[ -n $REPO && -n $VERSION ]] || { echo 'Specify --repo OWNER/REPO --version vX.Y.Z or --local-package PATH.' >&2; exit 2; }
  python3 "$HERE/release.py" manager --arch "$ARCH" --directory "$WORK/manager" --repo "$REPO" --version "$VERSION"
  PACKAGE=$WORK/manager/unpacked
fi
# Detect broken/partial installations instead of assuming absence.
LOAD=$(systemctl show "$UNIT" -p LoadState --value)
SB=$(command -v sing-box || true)
if [[ $LOAD == loaded ]]; then
  systemctl show "$UNIT" -p ExecStart -p WorkingDirectory > "$WORK/existing-service.txt"
  echo "Existing $UNIT preserved; helper discovers effective ExecStart and drop-ins."
elif [[ -n $SB || -e /etc/sing-box || -e /usr/local/bin/sing-box || -e /usr/bin/sing-box ]]; then
  echo 'Partial or custom sing-box installation detected. Specify --unit for an existing service or repair it first; nothing overwritten.' >&2; exit 1
else
  ((UPDATE_ONLY == 0)) || { echo 'Update never installs or upgrades sing-box.' >&2; exit 1; }
  [[ $UNIT == sing-box.service ]] || { echo 'Cannot create a custom sing-box unit automatically.' >&2; exit 1; }
  python3 "$HERE/release.py" singbox --arch "$ARCH" --directory "$WORK/singbox"
  SB_SOURCE="$WORK/singbox/unpacked/sing-box-1.14.4-linux-$ARCH/sing-box"
  "$SB_SOURCE" version
  install -m 0755 "$SB_SOURCE" /usr/local/bin/sing-box
  install -d -m 0700 /etc/sing-box /var/lib/sing-box
  printf '{"log":{"level":"info"},"inbounds":[],"outbounds":[{"type":"direct","tag":"direct"}]}\n' > /etc/sing-box/config.json
  /usr/local/bin/sing-box check -c /etc/sing-box/config.json
  cat > /etc/systemd/system/sing-box.service <<'UNIT'
[Unit]
Description=sing-box service
After=network.target
[Service]
WorkingDirectory=/var/lib/sing-box
ExecStart=/usr/local/bin/sing-box run -c /etc/sing-box/config.json
Restart=on-failure
LimitNOFILE=1048576
[Install]
WantedBy=multi-user.target
UNIT
  systemctl daemon-reload
  systemctl enable --now sing-box.service
fi
getent group sing-box-manager >/dev/null || groupadd --system sing-box-manager
id sing-box-manager >/dev/null 2>&1 || useradd --system --gid sing-box-manager --home-dir /var/lib/sing-box-manager --shell /usr/sbin/nologin sing-box-manager
[[ $(id -u sing-box-manager) != 0 ]] || { echo "Refusing root-valued service account" >&2; exit 1; }
install -d -o sing-box-manager -g sing-box-manager -m 0700 /var/lib/sing-box-manager
install -d -o root -g root -m 0700 /var/lib/sing-box-manager-helper
install -d -o root -g root -m 0755 "$DEST"
# Keep previous binary until replacement passes health verification.
if [[ -f $DEST/manager ]]; then cp -p "$DEST/manager" "$DEST/manager.previous"; fi
printf 'pending\n' > "$DEST/update-pending"
install -m 0755 "$PACKAGE/manager" "$DEST/manager.next"
mv -f "$DEST/manager.next" "$DEST/manager"
# Existing administrator-managed unit files are preserved on reruns.
for unit in sing-box-manager.service sing-box-manager-helper.service; do
  if [[ ! -f /etc/systemd/system/$unit ]]; then
    sed "s/--unit sing-box.service/--unit $UNIT/" "$PACKAGE/deploy/$unit" > "/etc/systemd/system/$unit"
    chmod 0644 "/etc/systemd/system/$unit"
  fi
done
rm -rf "$DEST/scripts.next"
mkdir "$DEST/scripts.next"
# Local builds can contain Python bytecode caches or other generated files.
# Install only the source scripts used by the runtime and maintenance commands.
cp "$PACKAGE/scripts/"*.sh "$PACKAGE/scripts/"*.py "$DEST/scripts.next/"
rm -rf "$DEST/scripts.previous"
if [[ -d $DEST/scripts ]]; then mv "$DEST/scripts" "$DEST/scripts.previous"; fi
mv "$DEST/scripts.next" "$DEST/scripts"
systemctl daemon-reload
failed=0
systemctl enable sing-box-manager-helper.service sing-box-manager.service || failed=1
systemctl restart sing-box-manager-helper.service sing-box-manager.service || failed=1
PORT=''
manager_healthy() {
  [[ $PORT =~ ^[0-9]+$ ]] || return 1
  systemctl is-active --quiet sing-box-manager-helper.service || return 1
  systemctl is-active --quiet sing-box-manager.service || return 1
  curl --fail --silent --max-time 2 "http://127.0.0.1:$PORT/healthz" | python3 -c 'import json,sys; sys.exit(0 if json.load(sys.stdin) == {"ok":True} else 1)' 2>/dev/null
}
for ((i=0;i<30;i++)); do
  PORT=$(python3 -c 'import json; print(json.load(open("/var/lib/sing-box-manager/selected-port.json"))["port"])' 2>/dev/null || true)
  if manager_healthy; then break; fi
  sleep 1
done
if ((failed)) || ! manager_healthy; then
  if [[ -f $DEST/manager.previous ]]; then
    cp -p "$DEST/manager.previous" "$DEST/manager.recovery"; mv -f "$DEST/manager.recovery" "$DEST/manager"
    systemctl reset-failed sing-box-manager-helper.service sing-box-manager.service || true
    systemctl restart sing-box-manager-helper.service sing-box-manager.service || true
  fi
  echo 'Manager health check failed. Previous binary restored when available; sing-box configuration preserved. Inspect journalctl -u sing-box-manager.' >&2
  exit 1
fi
rm "$DEST/update-pending"
# Read-only availability discovery always runs; HTTPS changes remain opt-in.
if command -v tailscale >/dev/null; then
  TS_STATE=$(timeout 5 tailscale status --json 2>/dev/null | python3 -c 'import json,sys; print(json.load(sys.stdin).get("BackendState","unavailable"))' 2>/dev/null || true)
  if [[ $TS_STATE == Running ]]; then
    printf 'Tailscale is running and authenticated. Optional private HTTPS setup: sudo bash /usr/local/lib/sing-box-manager/scripts/tailscale-setup.sh %s\n' "$PORT"
  else
    echo 'Tailscale is installed but not running/authenticated; localhost access retained. Authenticate before using --tailscale.'
  fi
else
  echo 'Tailscale is not installed; use the SSH forwarding command below for private access.'
fi
if ((TAILSCALE)); then bash "$HERE/tailscale-setup.sh" "$PORT" || echo 'Tailscale setup failed; localhost access remains available.'; fi
printf '\nManager healthy: http://127.0.0.1:%s\nSSH: ssh -L %s:127.0.0.1:%s YOUR_SERVER\nLogin token: sudo cat /var/lib/sing-box-manager/token\n' "$PORT" "$PORT" "$PORT"
systemctl --no-pager status sing-box-manager.service || true
