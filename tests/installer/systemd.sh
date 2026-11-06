#!/usr/bin/env bash
# Disposable Docker systemd integration; no host services or data are mounted.
set -euo pipefail
cd "$(dirname "$0")/../.."
NAME="sbm-installer-${RANDOM}-${RANDOM}"
WORK=$(mktemp -d)
trap 'docker exec "$NAME" journalctl --no-pager -u sing-box -u sing-box-manager-helper -n 60 2>/dev/null || true; docker rm -f "$NAME" >/dev/null 2>&1 || true; rm -rf "$WORK"' EXIT
ARCH=$(docker info --format '{{.Architecture}}')
case "$ARCH" in aarch64|arm64) ARCH=arm64;; x86_64|amd64) ARCH=amd64;; *) exit 2;; esac
(cd backend && CGO_ENABLED=0 GOOS=linux GOARCH="$ARCH" go build -o "$WORK/manager" ./cmd/manager)
cp -R scripts deploy "$WORK/"
# Python imports earlier in CI leave caches in local packages. They must neither
# abort installation nor become installed runtime files.
mkdir -p "$WORK/scripts/__pycache__"
printf 'fixture cache\n' > "$WORK/scripts/__pycache__/release.cpython-test.pyc"
cp tests/installer/config_transactions.py tests/installer/fake_health.py "$WORK/"
docker build -t sing-box-manager-installer-test tests/installer
docker run -d --name "$NAME" --privileged --cgroupns=private --tmpfs /run --tmpfs /run/lock sing-box-manager-installer-test
for ((i=0;i<20;i++)); do docker exec "$NAME" test -d /run/systemd/system && break; sleep 1; done
docker cp "$WORK/." "$NAME:/package"
docker exec "$NAME" bash /package/scripts/install.sh --local-package /package
docker exec "$NAME" test ! -e /usr/local/lib/sing-box-manager/scripts/__pycache__
docker exec "$NAME" sh -c 'test "$(systemctl is-active sing-box)" = active && test "$(systemctl is-active sing-box-manager)" = active && test "$(systemctl show sing-box-manager -p User --value)" = sing-box-manager'
docker exec "$NAME" sh -c 'sha256sum /etc/sing-box/config.json > /tmp/before; systemctl show sing-box -p MainPID --value > /tmp/pid'
docker exec "$NAME" bash /package/scripts/install.sh --local-package /package
docker exec "$NAME" sh -c 'sha256sum -c /tmp/before && test "$(cat /tmp/pid)" = "$(systemctl show sing-box -p MainPID --value)"'
# Port collision selects and persists a real bound fallback.
docker exec "$NAME" systemctl stop sing-box-manager
docker exec "$NAME" rm /var/lib/sing-box-manager/selected-port.json
docker exec -d "$NAME" python3 -m http.server 5030 --bind 127.0.0.1
docker exec "$NAME" systemctl start sing-box-manager
sleep 2
docker exec "$NAME" python3 -c 'import json; assert json.load(open("/var/lib/sing-box-manager/selected-port.json"))["port"] == 5031'
docker exec "$NAME" systemctl restart sing-box-manager
sleep 2
docker exec "$NAME" curl --fail http://127.0.0.1:5031/healthz
# Update never changes sing-box; failed replacement recovers the old Manager.
# Delay the broken process exit so Type=simple is deterministically active while
# the unrelated listener impersonates health on the stale selected port.
docker exec "$NAME" systemctl stop sing-box-manager
docker exec -d "$NAME" python3 /package/fake_health.py
sleep 1
docker exec "$NAME" curl --fail http://127.0.0.1:5031/healthz
docker exec "$NAME" sh -c 'cp /package/manager /package/manager.good; printf "#!/bin/sh\nsleep 5\nexit 1\n" > /package/manager; chmod +x /package/manager'
if docker exec "$NAME" bash /package/scripts/update.sh --local-package /package; then echo 'Expected failed update' >&2; exit 1; fi
docker exec "$NAME" sh -c 'cmp /usr/local/lib/sing-box-manager/manager /package/manager.good && sha256sum -c /tmp/before'
docker exec "$NAME" sh -c 'cp /package/manager.good /package/manager'
docker exec "$NAME" bash /package/scripts/update.sh --local-package /package
docker exec "$NAME" python3 /package/config_transactions.py
docker exec "$NAME" sh -c 'sha256sum /etc/sing-box/config.json > /tmp/before-uninstall'
docker exec "$NAME" sh -c 'printf "{}" > /var/lib/sing-box-manager-helper/transaction.json'
if docker exec "$NAME" bash /package/scripts/uninstall.sh; then echo "Uninstall must preserve pending recovery" >&2; exit 1; fi
docker exec "$NAME" sh -c 'test "$(systemctl is-active sing-box-manager-helper)" = active; rm /var/lib/sing-box-manager-helper/transaction.json'
docker exec "$NAME" bash /package/scripts/uninstall.sh
docker exec "$NAME" sh -c 'test "$(systemctl is-active sing-box)" = active && sha256sum -c /tmp/before-uninstall'
echo 'PASS: clean install, existing preservation, occupied port/reuse, failed update recovery, retry, uninstall preservation.'
