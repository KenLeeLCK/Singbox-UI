# Deployment and release operations

Production supports Debian 12+ and Ubuntu 22.04+ with systemd, on amd64 and arm64. The web process runs as `sing-box-manager`, with no Linux capabilities. A separate root helper exposes only fixed management operations over a group-restricted Unix socket. Its service arguments are the authority for the managed unit and file paths.

## Build and install a reviewed local checkout

```sh
bash scripts/build.sh
bash scripts/package.sh v0.1.0
# On the Linux server, unpack the matching archive:
tar xzf sing-box-manager-v0.1.0-linux-amd64.tar.gz -C /tmp/manager-release
sudo bash /tmp/manager-release/scripts/install.sh --local-package /tmp/manager-release
```

Create the extraction directory first. The `--local-package` route trusts your reviewed local build; downloaded releases always verify `SHA256SUMS`. Do not run the installer against an unrelated host for testing.

## Install a published release

After publishing your own GitHub release, download the tagged source archive over HTTPS, inspect it, and run its `scripts/install.sh`:

```sh
sudo bash scripts/install.sh --repo YOUR_ORG/YOUR_REPO --version v0.1.0 --tailscale
```

This is the one-command installation entry point from a reviewed checkout; no mutable remote shell script is executed. Required commands are listed in the install script; install missing distribution packages before retrying. Release assets are `sing-box-manager-vX.Y.Z-linux-{amd64,arm64}.tar.gz` and `SHA256SUMS`. No release has been published by this checkout.

A clean host receives official sing-box 1.14.4 binaries verified against SHA256 digests recorded from its upstream GitHub release, an empty inbound list, and direct outbound. No proxy or management listener is exposed publicly. A loaded existing service is preserved, and its effective systemd configuration is discovered by the helper. When the default unit is absent, the installer inspects effective `ExecStart` for installed and loaded service units. A unique service that directly executes a binary named `sing-box` is selected automatically, including binaries outside PATH and configuration paths supplied by drop-ins. Multiple candidates require `--unit custom-sing-box.service`; no service or configuration is modified while selection is ambiguous. A loaded default unit takes priority. Broken, masked, failed, or missing-executable installations are refused for repair. Shell wrappers and renamed executables are not guessed; configure a directly executable supported service explicitly. Partial or ambiguous installations fail without replacement. Existing configuration is never overwritten during Manager installation. The initial sing-box installation deliberately has monitoring disabled; enable it using a reviewed draft in the UI.

The Manager itself binds the first available socket beginning at 5030, persists its selection, and the installer reads `selected-port.json` only after starting the service. Existing persisted ports are reused where possible. The installer requires both Manager/helper services to be active and polls `/healthz` on the selected port for the exact expected JSON. Read the login token using `sudo cat /var/lib/sing-box-manager/token`; do not put it in a URL or shell history.

## Private HTTPS

The installer always reports read-only Tailscale availability and offers a private HTTPS setup command when authenticated. `--tailscale` additionally checks existing Serve configuration and CLI capability before opting into configuration. Automatic setup only runs when Serve has no existing configuration. It invokes `tailscale serve --bg --https=443 http://127.0.0.1:PORT`, never Funnel. Existing listeners/routes/services are left unchanged and require administrator integration. A 30-second timeout avoids indefinite interactive authorization waits. Tailscale must already be authenticated and HTTPS authorized in the tailnet. The final Tailscale status prints its private address.

Without Tailscale, use `ssh -L 5030:127.0.0.1:PORT YOUR_SERVER` and open `http://127.0.0.1:5030`. This avoids public exposure.

Reference: [official Tailscale Serve CLI](https://tailscale.com/docs/reference/tailscale-cli/serve).

## Updates and recovery

```sh
sudo bash /usr/local/lib/sing-box-manager/scripts/update.sh --repo YOUR_ORG/YOUR_REPO --version v0.1.1
```

Updates never install or upgrade sing-box. A lock prevents competing installers. Downloads and checksums complete before binary replacement. The previous Manager binary is retained as `manager.previous`; an `update-pending` record precedes atomic replacement. Failed startup restores the previous binary. If interrupted, the next installer invocation restores the previous binary before resuming. Existing unit files, settings, token, history and sing-box files are preserved. Administrative unit overrides may require explicit migration when upgrading across incompatible versions; inspect release notes.

Recovery without a download:

```sh
sudo cp /usr/local/lib/sing-box-manager/manager.previous /usr/local/lib/sing-box-manager/manager.recovery
sudo mv /usr/local/lib/sing-box-manager/manager.recovery /usr/local/lib/sing-box-manager/manager
sudo systemctl restart sing-box-manager-helper sing-box-manager
```

The helper service independently recovers pending configuration deployment transactions, including while the web process is stopped. Manager binary update recovery and sing-box configuration transaction recovery are separate mechanisms.

## Uninstall

```sh
sudo bash /usr/local/lib/sing-box-manager/scripts/uninstall.sh
```

Uninstallation refuses to remove the helper while a deployment transaction is unconfirmed, allowing recovery to finish first. Otherwise this removes only Manager binaries and its two units. sing-box, configuration, history/state, the dedicated service account, and Tailscale configuration remain. Inspect retained data and any dedicated Serve route before removing them manually.

## Publish

Set your intended Git remote, push the reviewed code, and push a semantic version tag. CI runs frontend lint/typecheck/tests/build, backend race tests, installer safety tests, Linux cross-compilation and packaging. The release workflow publishes archives/checksums on a `v*` tag. It never updates sing-box on any machine.

```sh
git remote add origin git@github.com:YOUR_ORG/YOUR_REPO.git
git add . && git commit -m 'Implement sing-box Manager'
git push -u origin main
git tag v0.1.0 && git push origin v0.1.0
```

## Isolated installer tests

`python3 tests/installer_test.py` tests checksum refusal, archive traversal/link refusal, safe extraction, shell syntax, mutable-version rejection, and preserving an existing Tailscale listener. Real systemd scenario testing uses the disposable image in `tests/installer/Dockerfile`; never bind-mount host `/etc`, `/var/lib` or `/run` into it. A Docker daemon is a test dependency only, never a production dependency. See `docs/installer-verification.md` for actual executed results and limitations.
