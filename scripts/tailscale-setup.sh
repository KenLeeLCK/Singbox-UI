#!/usr/bin/env bash
set -euo pipefail
PORT=${1:?Manager port required}
[[ $PORT =~ ^[0-9]+$ ]] && ((PORT>=5030 && PORT<=65535)) || exit 2
if ! command -v tailscale >/dev/null; then echo 'Tailscale unavailable; localhost access retained.'; exit 0; fi
status=$(tailscale status --json) || { echo 'Tailscale is not running; localhost access retained.'; exit 0; }
[[ $(python3 -c 'import json,sys; print(json.load(sys.stdin).get("BackendState", ""))' <<< "$status") == Running ]] || { echo 'Authenticate Tailscale first; localhost access retained.'; exit 0; }
config=$(tailscale serve status --json) || exit 1
# Never alter an existing listener, route, service or Funnel configuration.
if ! python3 -c 'import json,sys; c=json.load(sys.stdin); sys.exit(0 if not any(c.values()) else 1)' <<< "$config"; then
  echo 'Existing Tailscale Serve configuration detected; preserved unchanged. Configure a dedicated listener manually.'
  exit 0
fi
tailscale serve --help 2>&1 | grep -q -- '--https' || { echo 'Tailscale Serve CLI unsupported; preserved unchanged.'; exit 0; }
timeout 30 tailscale serve --bg --https=443 "http://127.0.0.1:$PORT"
tailscale serve status
