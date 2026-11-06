# Verification and compatibility boundaries

All results below refer to actual local execution on 2026-11-06, not a hosted GitHub workflow or production VPS. Tests use disposable directories, local-only listeners, command fixtures, and a disposable Debian systemd container. They do not replace or restart host services.

## Executed checks

| Area | Actual verification |
| --- | --- |
| Upstream binary | Official sing-box1.14.4 Darwin ARM64 release SHA256 matched GitHub asset digest; executable version/schema/check/run verified |
| Frontend | ESLint, TypeScript, production Vite build, generated1.14.4 schema/model regressions; exact results in frontend-verification.md |
| Go | Full `go test -race ./...` with both real-binary integration flags; `go vet ./...` |
| Native API | Real bearer-authenticated gRPC connection, connection tracking, controlled256KiB HTTP proxy transfer, actual byte counters, reset/gap tests, loopback restrictions |
| Configuration | Native schema/check, unknown field preservation, stale hash refusal, dependency/listener validation, multi-tag rule sets, history identifiers, JSONC read-only parsing |
| Metadata | Mode/owner/xattr preservation; Linux named POSIX ACL round-trip in Debian; refusal before replacement if metadata cannot be preserved |
| Security | Session/CSRF/origin checks, POST-only logout, redaction/ANSI stripping, stale monitoring preparation refusal, log subprocess cancellation/capacity |
| Installer | Real Debian12 ARM64 systemd clean install and rerun; unchanged existing config and process; fallback5030→5031 and persistence |
| Updates | Actual bad Manager binary startup rollback/retry; spoofed healthy HTTP listener cannot bypass inactive systemd services |
| Deployments | Real successful monitoring draft→validate→apply, native SSE, journald, history restore/apply; invalid config refusal; runtime bind-failure rollback; independent helper interrupted-transaction recovery |
| Removal | sing-box remains running with preserved configuration; uninstall refuses removal while a deployment recovery is pending |
| Custom installations | Unique custom service/non-PATH executable/drop-in working directory discovery; ambiguity refuses mutation |
| Downloads | Python tests for checksum refusal, mutable version refusal, archive traversal/link rejection and safe extraction |
| Tailscale | Isolated existing-listener preservation and capability/absence logic; no authenticated tailnet was changed |
| Dependency security | `govulncheck ./...`: no vulnerabilities found after patch upgrades; `npm audit`: zero vulnerabilities |

Browser integration passed after fixing Monaco model ownership and upgrading its cancellation handling; the final theme/repeated-navigation run also passed, as recorded below. It launches temporary real Manager/sing-box processes and covers real telemetry, schema CRUD, visual route/rule-set editing, duplicate/reorder persistence, dependency refusal, invalid JSON protection, Monaco/diff, log controls, external conflicts, desktop/light/mobile layouts and JavaScript exception checks. It never applies host configuration.

## Reproduce

```sh
npm --prefix frontend ci
npm --prefix frontend run lint
npm --prefix frontend run typecheck
npm --prefix frontend test
npm --prefix frontend run build
(cd backend && go test -race ./... && go vet ./...)
python3 tests/installer_test.py
bash tests/installer/systemd.sh  # disposable privileged Docker container
```

Use both variable names to include all actual binary integrations:

```sh
cd backend
SING_BOX_BIN=/absolute/path/to/sing-box \
SING_BOX_TEST_BINARY=/absolute/path/to/sing-box go test -race -v ./...
```

Browser suite (after a current frontend/backend build):

```sh
npm --prefix tests/browser ci
npx --prefix tests/browser playwright install chromium
SING_BOX_BIN=/absolute/path/to/sing-box npm --prefix tests/browser test
```

On Linux CI, `scripts/ci-browser.sh` fetches the checksum-pinned upstream test binary, runs real Go integration tests and launches the browser suite. All build dependencies are development-only.

## Remaining boundaries

- No production VPS was accessed, and no GitHub repository/release was published. Remote access, target repository and publishing credentials were not supplied.
- Real Tailscale authentication/HTTPS provisioning was not exercised. Existing Serve configuration is preserved by refusal, requiring manual integration for shared listeners.
- Real systemd installation tests ran on Debian12 ARM64. Linux amd64 binaries are cross-compiled; Ubuntu and amd64 runtime behavior were not independently exercised. Deployment scripts gate to Debian/Ubuntu and amd64/arm64.
- Multiple-source, directory-based, JSONC and ambiguous service-command configurations are inspected conservatively without visual write-back. Use external file-level editing for these layouts; the Manager will not silently flatten or strip comments.
- Arbitrary schema maps and deeply nested uncommon fields use JSON input. Complex protocol changes can leave incompatible old fields; this is visible and must pass the installed checker before application.
- Health checks verify systemd, known TCP listeners and configured native API authentication. They do not prove all remote outbounds/DNS/UDP/client credentials work. The isolated controlled HTTP proxy integration is not a production network certification.
- Linux ACL/xattr preservation is tested. An enforcing SELinux deployment was not available; failures to preserve required metadata refuse replacement.
- No older/newer sing-box binary was used for runtime verification. Schema and capability detection support graceful degradation; version1.14.4 is the verified target.
- Local macOS mode deliberately supports inspection/drafts/checking, while Linux helper apply and journald operate only in systemd deployments.
- The embedded Monaco bundle is relatively large. It is fully local; no external editor/CDN is needed. Interface snapshots use real idle native telemetry, not synthetic traffic.

Detailed subsystem evidence: [backend](backend-verification.md), [frontend](frontend-verification.md), [installer](installer-verification.md), [native monitoring](telemetry.md).

## Final integrated results

Final clean lockfile build succeeded with `bash scripts/build.sh`. A transient npm ENOTEMPTY cache-removal failure was resolved by moving the generated `node_modules` directory aside and installing afresh; the sequential rerun passed.

- Frontend ESLint and TypeScript: PASS.
- Vitest: 2 test files, **11 tests passed**.
- Real Chromium smoke from the final binary: **PASS** every assertion, including three repeated Monaco/diff lifecycle cycles and zero uncaught browser exceptions. Final output: `.local/browser-final.log` (local ignored evidence).
- Actual-binary Go race tests: API1.966s, core15.528s, telemetry4.598s; all packages PASS. `go vet`: PASS. `go mod verify`: all modules verified.
- `govulncheck`: **No vulnerabilities found**. Frontend `npm audit`: **0 vulnerabilities**.
- Installer safety tests: **9 passed**. Actual Debian systemd full/custom installation scenarios: **PASS**, containers cleaned up.
- Desktop, native light theme, mobile dashboard and mobile form snapshots were captured and visually inspected. Charts show real idle counters; no demo telemetry is represented as production data.

The final Monaco0.57 bundle produces an initial application chunk around1.13MB gzip. This known build-size warning does not affect native polling correctness; workers and editor assets are embedded locally. A custom lifecycle owns and disposes editor models in the correct order rather than relying on the faulty older React wrapper. No browser exceptions are suppressed.
