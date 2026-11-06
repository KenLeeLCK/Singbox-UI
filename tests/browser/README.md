# Real application browser regression

This test owns all its files and processes. It creates a temporary native sing-box configuration with no public listeners, starts a disposable sing-box API and a local-mode Manager, reads the temporary token internally, exercises the UI and then terminates both processes and deletes the temporary files. It does not target an existing Manager or host systemd service.

Build the current frontend and `artifacts/manager` first, then:

```sh
npm --prefix tests/browser ci
npx --prefix tests/browser playwright install chromium
SING_BOX_BIN=/absolute/path/to/verified/sing-box npm --prefix tests/browser test
```

Set `MANAGER_BIN` to test a different build. The sing-box binary must support the native1.14 API and schema. Use the official checksum-verified release. `scripts/ci-browser.sh` prepares the Linux CI binary and browser dependencies without installing host services.

Assertions cover actual authenticated telemetry, inbound create/edit and native validation, outbound/visual routing/inline rule-set CRUD, persisted duplicate/reorder results, referenced outbound deletion refusal, invalid JSON preservation, Monaco editor/diff and repeated lifecycle navigation, log controls, external file conflict refusal/reconciliation, light/dark and mobile layouts, and uncaught browser exceptions. Screenshots use real idle telemetry and are saved under `.local/browser-artifacts/` by default. Set `UPDATE_SCREENSHOTS=1` explicitly to refresh `docs/screenshots`. Failures also retain a DOM snapshot, browser/process events and a Playwright trace.

The suite also runs authentication/recovery over HTTP and an isolated HTTPS reverse proxy with Tailscale Serve's Host-preserving behavior. It checks cookie flags, invalid credentials, reload persistence, initial and post-login configuration failures, optional status failure, session expiry, SSE interruption, logout invalidation and reauthentication. The full editor/navigation suite runs over HTTPS as well. OpenSSL is required for the ephemeral test certificate; only isolated contexts ignore certificate errors.

Failure traces retain DOM/network diagnostics after replacing generated tokens and session cookie values. Trace screenshots are disabled because image bytes cannot reliably be redacted. Never use these test harnesses against production.

## Dashboard activity regression

`node tests/browser/activity.mjs` serves the production frontend build with synthetic telemetry on an ephemeral loopback port. It checks newest-first ordering, ten-row previews, hidden counts, keyboard controls, a 480px expanded viewport, live scroll anchoring when rows arrive/close, retained state across empty feeds, and unchanged row DOM for counter-only updates with hundreds of connections. Desktop and mobile layouts, including reduced motion, are covered. It uses no production credentials or services and runs as part of the browser test command. Build the frontend first.
