# Development and packaging

Singbox-UI uses React/TypeScript/Vite and a Go backend with embedded frontend assets. Build tools are needed on the development machine, not on a VPS using a prebuilt package.

## Prerequisites

- Node.js 22 and npm.
- Go 1.27.3 or the version required by `backend/go.mod`.
- A verified sing-box 1.14 binary for schema, validation and browser integration tests.
- Docker for the isolated Linux/systemd integration suites.

## Checkout and build

```bash
git clone https://github.com/KenLeeLCK/Singbox-UI.git
cd Singbox-UI
bash scripts/build.sh
```

This installs the pinned frontend dependencies, applies the tracked Monaco dependency patch, builds assets into `backend/internal/web/dist`, and produces `artifacts/manager` for the build host. Keep `frontend/patches` with the checkout; do not skip install scripts.

## Local inspection

After building, supply paths to your own configuration and sing-box binary:

```bash
make dev CONFIG=/path/to/config.json SINGBOX=/path/to/sing-box
```

Local mode supports inspection, validation and drafts; it does not restart a system service. The process prints its loopback address. Its state and access token are stored under `.local/manager`; use that token to unlock the UI. Linux systemd deployment and journald functionality require the installed helper.

For frontend editing, run `npm --prefix frontend run dev` in another terminal. Vite proxies `/api` to `127.0.0.1:5030`; if the backend selected a fallback port, adjust the development proxy accordingly.

## Build Linux installation packages

From the repository root, after the frontend build:

```bash
bash scripts/package.sh v0.1.0-local
```

This example is a **local package label**, not a published Release. Outputs:

- `artifacts/sing-box-manager-v0.1.0-local-linux-amd64.tar.gz`
- `artifacts/sing-box-manager-v0.1.0-local-linux-arm64.tar.gz`
- `artifacts/SHA256SUMS`

Each archive includes the Linux `manager` executable, `scripts/`, `deploy/`, documentation and license. Copy the matching archive and checksum file to the VPS, then follow the [README installation steps](../README.md#installation). Do not pass a raw source checkout to `--local-package`: it requires the executable at the package root.

## Tests

```bash
npm --prefix frontend run lint
npm --prefix frontend run typecheck
npm --prefix frontend test
(cd backend && go test -race ./...)
python3 tests/installer_test.py
```

The Go race detector needs a working C compiler. For Linux browser integration, build first, then run:

```bash
bash scripts/ci-browser.sh
```

This downloads and verifies the pinned Linux sing-box binary, builds the Manager, installs browser dependencies, and runs real integration tests with native and controlled deferred idle scheduling. It uses isolated configurations and loopback listeners. For other hosts, see the [browser test guide](../tests/browser/README.md).

Browser failures retain screenshots, DOM, console/network events and Playwright traces under `.local/browser-artifacts/`. `UPDATE_SCREENSHOTS=1` explicitly refreshes the documentation screenshots; normal runs leave them unchanged.

Run installation, update, rollback and custom-service discovery checks only in the provided disposable containers:

```bash
bash tests/installer/systemd.sh
bash tests/installer/custom-service.sh
```

Do not mount production service/configuration directories into these fixtures. See [installer verification](installer-verification.md), [frontend verification](frontend-verification.md) and [backend verification](backend-verification.md) for coverage and limitations.

## Further documentation

- [Deployment and recovery](deployment.md)
- [Architecture](architecture.md) and [API](api.md)
- [Native telemetry](telemetry.md)
- [Release maintainer guide](releasing.md)

Drafts, external-change conflicts, installed-binary validation and deployment rollback remain part of the same native JSON configuration workflow. Preserve unknown fields and source boundaries when changing editors; multi-file, JSONC and ambiguous imports remain read-only.
