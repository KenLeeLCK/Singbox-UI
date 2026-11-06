# Installer verification evidence

Test host: macOS arm64, Docker Desktop server 29.8.0. Production tests execute only inside disposable Debian 12 arm64 containers with actual systemd 252, never against the host's services.

## Executed

- `python3 tests/installer_test.py`: nine tests pass (checksum/mismatch, traversal and symlink rejection, extraction, shell syntax, existing Tailscale listener preserved, mutable release version rejected, direct executable detection, loaded default priority, broken executable refusal).
- `docker build -t sing-box-manager-installer-test tests/installer`: succeeded. Base Debian image pinned by manifest digest.
- `bash tests/installer/systemd.sh`: initial installation-only suite passed: official verified sing-box 1.14.4 download/clean installation; dedicated nonroot Manager and separate root helper startup; idempotent rerun preserving configuration SHA256 and sing-box process ID; occupied 5030 selects and persists 5031; corrupt Manager binary update restores previous binary; retry succeeds; uninstall preserves running sing-box and configuration; pending unconfirmed deployment refuses helper removal.
- Expanded real API deployment suite initially found a systemd start-limit rollback defect. The backend was corrected to reset the configured unit failure counter before recovery. The complete suite was rebuilt and rerun successfully: invalid config refused without live replacement; actual mixed inbound runtime bind failure rolled back and restored active sing-box; independently restarting the helper recovered a persistent interrupted transaction. Final execution: exit 0, all scenario assertions passed. Final expanded monitoring/health-spoof run also completed with exit 0. Raw local output: `.local/installer-systemd.log` (ignored from Git).
- Expanded monitoring integration also verifies native API draft preparation, validation and successful safe apply to loopback 9095; authenticated named telemetry SSE reports available; actual journald SSE contains a startup entry and releases its follower process on disconnect; history restores a draft without changing live state and applies the original configuration through the normal deployment lifecycle.
- Installer health requires both service units active and the exact expected health JSON; a real unrelated fixture listener returning the exact `{"ok":true}` health body on the stale selected port could not satisfy checks while the services failed. The previous binary recovered and selected the next available port.
- Latest integration build includes patched Go dependencies and Go 1.27.3. A separate unused exploratory container was removed; test containers clean themselves up.

## Files and references inspected

Official sing-box v1.14.4 GitHub release asset metadata and SHA256 digests; official Tailscale Serve CLI docs; backend CLI and transaction recovery contract. Files created/changed: `scripts/`, `deploy/`, `.github/workflows/`, `tests/installer_test.py`, `tests/installer/`, `Makefile`, `.gitignore`, `LICENSE`, `docs/deployment.md` and this evidence record.

## Custom installation discovery verification

`bash tests/installer/custom-service.sh` completed with exit 0 in an actual Debian 12 ARM64 systemd container. It installed an existing `proxy-engine.service` whose binary was `/opt/custom-engine/bin/sing-box` (absent from PATH), and whose effective drop-in selected `production.json` relative to `/srv/proxy-config`. Two direct candidate units were refused before Manager filesystem mutation. Removing the extra candidate allowed automatic discovery and installation. Authenticated API inspection confirmed the correct source file, configuration tag, custom unit and running status. Configuration SHA256 and existing process ID remained unchanged across initial install and rerun. Raw output: `.local/installer-custom.log`.

The final full systemd scenario suite was also rerun after discovery changes and completed with exit 0. Both installation paths now report Tailscale availability read-only even without the opt-in setup flag. No test containers remain running.

## Limits

No live Tailscale account or production VPS was changed. Tailscale conflict refusal is exercised with a deterministic CLI fixture; real tailnet certificate/Serve setup remains unverified. Linux amd64 is cross-compiled but this local Docker host executes arm64. Ubuntu has not been booted for installer validation. GitHub release downloads for this new Manager cannot be exercised before a real repository/release exists; release checksum/extraction logic is tested locally. No release has been published.

Automatic discovery deliberately excludes shell wrappers and executables renamed away from `sing-box`; no wrapper semantics are guessed. Ambiguous services require explicit selection; broken matching services require repair.
