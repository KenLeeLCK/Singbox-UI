# Singbox-UI

A modern, Apple Liquid Glass-inspired web interface for managing sing-box on your Linux VPS. Monitor traffic, edit your configuration visually, and review changes before deploying them—all through a private dashboard.

## Features

- Real-time upload/download traffic monitoring and connection details.
- Visual inbound, outbound, routing rule and rule-set management.
- Live sing-box system logs with search and filtering.
- Advanced JSON editing, validation and configuration diffs.
- Automatic detection and import of existing sing-box configurations.
- Configuration backups, deployment health checks and rollback.
- Private HTTPS access through Tailscale, with SSH tunneling as an alternative.

## Screenshots

![Desktop overview with real idle native monitoring](docs/screenshots/overview-desktop.png)

[Mobile preview](docs/screenshots/overview-mobile.png) · [Light appearance](docs/screenshots/overview-light.png)

## Installation

**Requirements:** Debian 12+ or Ubuntu 22.04+ with systemd, an amd64 or arm64 CPU, and sudo/root access. Node.js and Go are not needed on the VPS when installing a built package.

**Release availability:** no prebuilt GitHub Releases are published as of November 6, 2026. For now, use the [source build and packaging guide](docs/development.md) and transfer the matching archive plus `SHA256SUMS` to your VPS. A Git clone or GitHub source ZIP alone is not an installable package.

Once a release is published, download these two files from [GitHub Releases](https://github.com/KenLeeLCK/Singbox-UI/releases) into a new directory on the VPS:

- `sing-box-manager-<version>-linux-amd64.tar.gz` for x86-64, or `sing-box-manager-<version>-linux-arm64.tar.gz` for ARM64.
- `SHA256SUMS` from the same release.

For either a release archive or a locally built archive, install prerequisites, verify the checksum, extract and run the installer from that directory. Keep only the one matching archive there:

```bash
sudo apt-get update && sudo apt-get install -y ca-certificates curl python3
sha256sum --check --ignore-missing SHA256SUMS &&
mkdir singbox-ui &&
tar -xzf sing-box-manager-*-linux-*.tar.gz -C singbox-ui &&
cd singbox-ui &&
sudo bash scripts/install.sh --local-package "$PWD"
```

If Tailscale is already installed and authenticated, add `--tailscale` to the final command to request private HTTPS setup automatically.

The installer detects existing sing-box services. On a clean VPS without sing-box, it installs the checksum-verified sing-box 1.14.4 release. It prints the selected dashboard address and access instructions when finished.

## Existing sing-box configuration import

Existing proxy nodes, settings and configuration files are preserved. Installing Singbox-UI does **not** replace, upgrade or restart an existing sing-box service. Its effective systemd configuration, including drop-ins, is used to locate and import the configuration into the dashboard.

A unique custom service is detected automatically. If several services are found, select one explicitly by adding `--unit custom.service` to the install command. Broken or unsupported installations require administrator attention rather than being overwritten.

Single-file strict JSON configurations can be managed through the UI. Multi-file, JSONC and ambiguous configurations remain read-only. Saving a draft does not restart sing-box; applying a reviewed configuration does.

## Accessing the dashboard

The default address is **`127.0.0.1:5030`**. If occupied, Singbox-UI selects an available port through 5099 and remembers it. On the VPS, find the selected port and login token with:

```bash
sudo cat /var/lib/sing-box-manager/selected-port.json
sudo cat /var/lib/sing-box-manager/token
```

Enter the token on the dashboard's login screen.

### Tailscale: private HTTPS

Tailscale is the recommended way to access the dashboard without exposing it publicly. With Tailscale already installed, authenticated and HTTPS enabled for your tailnet, run on the VPS:

```bash
manager_port=$(sudo python3 -c 'import json; print(json.load(open("/var/lib/sing-box-manager/selected-port.json"))["port"])')
sudo bash /usr/local/lib/sing-box-manager/scripts/tailscale-setup.sh "$manager_port"
```

Open the private HTTPS URL printed after successful setup from a device on your tailnet. Existing Tailscale Serve configurations are preserved; if one exists, the script leaves it unchanged and asks you to integrate the dashboard manually. See [access and deployment details](docs/deployment.md).

### SSH tunnel

Alternatively, run this on your own computer, replacing `USER@SERVER` and the **remote** port if it differs from 5030:

```bash
ssh -N -L 5030:127.0.0.1:5030 USER@SERVER
```

Open `http://127.0.0.1:5030` locally. No public management port is required.

## Updating and uninstalling

After an updated release is published, select its exact tag from [Releases](https://github.com/KenLeeLCK/Singbox-UI/releases) and run on the VPS:

```bash
read -r -p 'Published release tag: ' release_tag
sudo bash /usr/local/lib/sing-box-manager/scripts/update.sh \
  --repo KenLeeLCK/Singbox-UI --version "$release_tag"
```

For a locally built update, extract the new package and use `sudo bash scripts/update.sh --local-package "$PWD"` from its directory. Updates change Singbox-UI only; they do not upgrade sing-box.

To uninstall:

```bash
sudo bash /usr/local/lib/sing-box-manager/scripts/uninstall.sh
```

Uninstall preserves sing-box, its configuration, Manager history/state, the service account and Tailscale routes. An unconfirmed deployment must recover before the helper can be removed. See [deployment operations](docs/deployment.md) for recovery and retained-data details.

## Notes and compatibility

- Targets sing-box 1.14; available configuration fields depend on the installed binary's schema and validator.
- Traffic monitoring requires the native API. **Prepare monitoring** creates a draft for review and application; import alone does not enable it or restart sing-box.
- Live system logs require systemd/journald. Configuration access should be limited to trusted administrators.
- For building and contributing, see [development](docs/development.md). Detailed behavior is documented in [deployment](docs/deployment.md) and [architecture](docs/architecture.md).

## License

[GPL-3.0-or-later](LICENSE). The sing-box protobuf wire definitions are derived from SagerNet/sing-box under the same license.
