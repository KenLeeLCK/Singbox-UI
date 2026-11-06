#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/../.."
NAME="sbm-custom-${RANDOM}-${RANDOM}"
WORK=$(mktemp -d)
trap 'docker exec "$NAME" journalctl --no-pager -u proxy-engine -u sing-box-manager-helper -n 30 2>/dev/null || true; docker rm -f "$NAME" >/dev/null 2>&1 || true; rm -rf "$WORK"' EXIT
ARCH=$(docker info --format '{{.Architecture}}')
case "$ARCH" in aarch64|arm64) ARCH=arm64;; x86_64|amd64) ARCH=amd64;; *) exit 2;; esac
(cd backend && CGO_ENABLED=0 GOOS=linux GOARCH="$ARCH" go build -o "$WORK/manager" ./cmd/manager)
cp -R scripts deploy "$WORK/"
docker build -t sing-box-manager-installer-test tests/installer
docker run -d --name "$NAME" --privileged --cgroupns=private --tmpfs /run --tmpfs /run/lock sing-box-manager-installer-test
for ((i=0;i<20;i++)); do docker exec "$NAME" test -d /run/systemd/system && break; sleep 1; done
docker cp "$WORK/." "$NAME:/package"
docker exec "$NAME" python3 /package/scripts/release.py singbox --arch "$ARCH" --directory /tmp/upstream
docker exec -i "$NAME" bash -s -- "$ARCH" <<'SETUP'
set -euo pipefail
mkdir -p /opt/custom-engine/bin /srv/proxy-config /etc/systemd/system/proxy-engine.service.d
cp "/tmp/upstream/unpacked/sing-box-1.14.4-linux-$1/sing-box" /opt/custom-engine/bin/sing-box
chmod 0755 /opt/custom-engine/bin/sing-box
printf '{"inbounds":[],"outbounds":[{"type":"direct","tag":"preserve-me"}]}\n' > /srv/proxy-config/production.json
chmod 0600 /srv/proxy-config/production.json
cat > /etc/systemd/system/proxy-engine.service <<'UNIT'
[Unit]
Description=Custom existing proxy
[Service]
ExecStart=/opt/custom-engine/bin/sing-box run -c /intentionally/not-used.json
[Install]
WantedBy=multi-user.target
UNIT
cat > /etc/systemd/system/proxy-engine.service.d/custom.conf <<'UNIT'
[Service]
ExecStart=
ExecStart=/opt/custom-engine/bin/sing-box run -c production.json
WorkingDirectory=/srv/proxy-config
UNIT
systemctl daemon-reload
systemctl start proxy-engine
sha256sum /srv/proxy-config/production.json > /tmp/config-before
systemctl show proxy-engine -p MainPID --value > /tmp/pid-before
! command -v sing-box
# A second direct service makes automatic selection ambiguous.
cat > /etc/systemd/system/other-proxy.service <<'UNIT'
[Service]
ExecStart=/opt/custom-engine/bin/sing-box run -c /srv/proxy-config/production.json
UNIT
systemctl daemon-reload
SETUP
if docker exec "$NAME" bash /package/scripts/install.sh --local-package /package; then echo 'Ambiguous services must be rejected' >&2; exit 1; fi
docker exec "$NAME" sh -c 'test ! -e /usr/local/lib/sing-box-manager; sha256sum -c /tmp/config-before; test "$(cat /tmp/pid-before)" = "$(systemctl show proxy-engine -p MainPID --value)"; rm /etc/systemd/system/other-proxy.service; systemctl daemon-reload'
docker exec "$NAME" bash /package/scripts/install.sh --local-package /package
docker exec -i "$NAME" python3 - <<'CHECK'
import http.cookiejar,json,pathlib,subprocess,urllib.request
state=pathlib.Path('/var/lib/sing-box-manager')
port=json.loads((state/'selected-port.json').read_text())['port'];base=f'http://127.0.0.1:{port}'
c=urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))
req=urllib.request.Request(base+'/api/login',data=json.dumps({'token':(state/'token').read_text().strip()}).encode(),headers={'Content-Type':'application/json','X-Requested-With':'sing-box-manager'})
c.open(req).close()
with c.open(base+'/api/config') as response: config=json.load(response)
assert config['sources']==['/srv/proxy-config/production.json'],config['sources']
assert config['config']['outbounds'][0]['tag']=='preserve-me'
assert not config['readOnly']
with c.open(base+'/api/status') as response: status=json.load(response)
assert status['unit']=='proxy-engine.service' and status['running'],status
print('PASS: auto-discovered custom service, non-PATH binary, effective drop-in and relative config import')
CHECK
docker exec "$NAME" sh -c 'sha256sum -c /tmp/config-before; test "$(cat /tmp/pid-before)" = "$(systemctl show proxy-engine -p MainPID --value)"'
docker exec "$NAME" bash /package/scripts/install.sh --local-package /package
docker exec "$NAME" sh -c 'sha256sum -c /tmp/config-before; test "$(cat /tmp/pid-before)" = "$(systemctl show proxy-engine -p MainPID --value)"'
echo 'PASS: ambiguous services rejected without mutation; custom existing process/config preserved across two installs'
