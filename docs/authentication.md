# Token-only authentication

Caddy terminates public HTTPS and preserves Host; Manager stays on 127.0.0.1. The privileged helper and native sing-box monitoring API are never reverse-proxied directly. The static login shell and minimal `/healthz` are public. Every configuration, status, schema, history, log, telemetry and mutation API requires a server-side session.

Manager generates 32 random bytes (256 bits) using crypto/rand, stores the hex token in an owner-only regular file, and compares SHA-256 digests in constant time. Existing tokens are preserved. Startup refuses insecure file permissions or tokens shorter than 32 characters; format/length alone cannot prove the entropy of a manually supplied token. Tokens belong only in the JSON login body, never URLs, assets or logs.

## Failed login protection

`--login-attempts` (default 10) and `--login-window` (default 1m) define a global failure budget for the single administrator account. Reservations occur before reading request bodies, so concurrent requests cannot bypass the budget. Successful logins refund their reservations; malformed requests count as failures. Blocked attempts return 429 with Retry-After and do not extend the fixed window. Existing authenticated sessions continue working during login throttling.

The budget is shared across all source addresses and access paths, including Caddy, Tailscale and SSH tunnels. Forwarded, X-Forwarded-For, X-Real-IP and X-Forwarded-Proto are not trusted. No per-IP table can be filled or evaded using forged proxy headers. A sustained attack can still delay new administrator logins at each window boundary; there is no permanent account lockout. This is not volumetric DDoS protection.

Login bodies are limited to 1 KiB and five seconds, with one JSON value. The web listener accepts at most 256 concurrent connections, has 16 KiB maximum headers, a five-second header deadline and a 15-second request read deadline. Rejected API requests do not reach the helper. Keep upstream network protections appropriate to the host's exposure.

## Sessions and CSRF

Each login generates a new 256-bit identifier and invalidates the presented old session. At most 32 sessions are retained; the oldest is revoked when necessary. Absolute lifetime defaults to eight hours (`--session-ttl`, maximum 24h). Expiry and logout cancel active telemetry/log streams as well as rejecting subsequent API requests. Restarting Manager invalidates all sessions, but does not rotate the login token.

Cookies are HttpOnly, SameSite=Strict and Path=/. Public Host names/IPs always receive Secure cookies, even if a non-browser request omits Origin. Explicit localhost/loopback HTTP access supports SSH tunneling without Secure; HTTPS Origin or direct TLS always enables Secure. Forwarded headers never determine this decision.

Origin must be an exact HTTP(S) origin matching the preserved Host, without userinfo, path, query or fragment. Public-host HTTP Origins and cross-site Fetch Metadata are rejected. All mutations, including login/logout, need an allowed Origin or the non-browser X-Requested-With: sing-box-manager header. There is no permissive CORS response. Authenticated browser login behind Caddy and Tailscale uses its actual HTTPS Origin.

## HTTPS and browser headers

Keep Caddy's existing externally managed certificate, loopback upstream, dedicated public bind and HTTP-01 renewal setup. Remove only the endpoint's basic_auth block after the Manager tests pass. No new authentication provider is required. Retain Authorization stripping at the upstream boundary; Manager authenticates with its session cookie.

The app sends nosniff, no-referrer, DENY framing protection, and a self-only CSP with frame-ancestors/base-uri/object-src restrictions. Inline styles and blob workers remain allowed for Monaco; scripts and connections remain same-origin. Public API responses are no-store. Caddy's modern TLS defaults remain in effect.

HSTS is deliberately not relied on for the public IP: browsers cannot treat IP addresses as HSTS hosts ([MDN](https://developer.mozilla.org/en-US/docs/Web/HTTP/Reference/Headers/Strict-Transport-Security)). The public port accepts HTTPS only. Keep using the explicit HTTPS URL; no certificate verification bypass is acceptable.

## Verification and recovery

`tests/browser/security.mjs` runs disposable HTTP and HTTPS servers with short configured limits to test real expiry, temporary lockout/recovery, forged forwarding headers, API denial, cookie flags, CSRF, logout and the token-only login shell. The existing auth and editor suites verify full application/CSP compatibility. Backend tests cover concurrency, session limits/rotation and revocation of open streams.

Before production activation, retain root-private copies of the Manager binary, Caddyfile, relevant settings and certificate/renewal metadata. Replace only Manager, validate Caddy, reload it gracefully, then test public HTTPS, SSH-tunneled HTTP and private Tailscale HTTPS. A detached deployment supervisor must restore both Manager and Caddy if health/security checks fail or external verification is not confirmed. Never restart sing-box or change proxy configuration to deploy authentication updates.
