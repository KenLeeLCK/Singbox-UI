# Authentication and private HTTPS loading verification

## Findings (2026-11-06)

The existing SSH tunnel owns IPv6 `[::1]:5030`; an unrelated local Manager owns IPv4 `127.0.0.1:5030`. Chromium successfully authenticated to the unmodified VPS over the IPv6 tunnel, stored its HttpOnly session cookie, received 200 from config/status/schema/history, and rendered the dashboard. The production token is rejected with 401 by the unrelated IPv4 local Manager. Use a dedicated SSH forward (this session uses 127.0.0.1:15030) to avoid the collision. The original failure against the actual VPS over SSH was not reproduced.

Production Tailscale Serve uses `/` on HTTPS with a TCP HTTP target at `127.0.0.1:5030`. HTTPS navigation stalled before the login UI. A separate timed request downloaded only 151,191 bytes of the approximately 4.1 MB JavaScript entry in 20 seconds. Eager Monaco loading and uncompressed assets made startup dependent on a large transfer. Network throughput is an observed contributing condition, not an authentication defect.

The corrected entry is approximately 315 KB raw / 106 KB gzip, with approximately 7.5 KB compressed initial CSS. Monaco and its styles load on demand. Static responses support cached gzip, immutable hashed assets, revalidated HTML, and missing-asset 404s. Only the read-only schema API is also compressed; configuration, credential and SSE responses are excluded. A static startup panel remains visible during slow JavaScript transfer.

Production HTTPS timing also reproduced a dashboard wait behind optional schema/history requests. The frontend now renders immediately after a successful required configuration load, loads optional data in the background, rejects stale loads, uses status-based authentication errors, provides retry after failed loads, tolerates unavailable optional endpoints, handles expiry during polling/actions, and exposes a rendering recovery boundary. API reads have bounded waits; deployment mutations retain a longer budget for validation/health/rollback. SSE reconnects without forcing logout.

Tailscale's TCP reverse proxy preserves Host ([upstream implementation](https://github.com/tailscale/tailscale/blob/main/ipn/ipnlocal/serve.go)). No forwarded headers are trusted. Same-host Origin validation, CSRF checks, HttpOnly/SameSite cookies and HTTPS Secure cookies remain enabled. Logout retains the appropriate Secure flag, expired sessions are removed, and random session generation errors are checked. Host-rewriting proxies are intentionally unsupported without an explicit trust model.

## Verification

Run `npm --prefix frontend run lint`, `npm --prefix frontend run typecheck`, `npm --prefix frontend test`, `npm --prefix frontend run build`, and `go test -race ./...` from backend. Run `python3 tests/installer_test.py`. The browser suite requires a built Manager and `SING_BOX_BIN`; `npm --prefix tests/browser test` exercises HTTP/HTTPS authentication and failure recovery plus native, deferred-editor-idle and HTTPS full UI scenarios. CI additionally executes Linux systemd installation, deployment rollback and custom-service scenarios.

Tests use temporary configurations and synthetic credentials. Failed traces are sanitized before retention; trace screenshots are disabled. Production browser checks must never record credentials, response bodies or authenticated traces.

## Safe Manager-only update and rollback

Build and test the exact revision, including CI, before uploading a Linux binary matching the VPS architecture. Record the existing sing-box PID/start time, checksums of `/etc/sing-box`, and `tailscale serve status --json`. Back up the installed Manager binary and Manager service units into a root-only directory. Upload the candidate to a staging path, verify SHA-256 on both sides, install it atomically at the existing Manager path, and restart only `sing-box-manager-helper.service` and `sing-box-manager.service`.

Verify both services, SSH, localhost and actual Tailscale browser login, authenticated reload, cookies, APIs and dashboard. Compare sing-box PID/start time/config checksums and Serve configuration with the baseline. On failure, restore the backed-up Manager binary atomically and restart only those two Manager services. Do not run install/reset/Funnel commands, change proxy configuration or restart sing-box. A versioned release is still needed to distribute the corrected binary through the normal release updater; do not overwrite an existing release.
