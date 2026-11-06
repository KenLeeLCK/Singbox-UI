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
