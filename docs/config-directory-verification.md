# Single-file configuration directories

Manager supports a direct `sing-box -D /var/lib/sing-box -C /etc/sing-box run` service when the directory has exactly one effective regular `.json` file containing strict JSON. `config.json.bak` does not count. Multiple sources, mixed/repeated selectors, symlink layouts, missing directories, JSONC content, wrappers, malformed arguments and unknown flags remain protected.

## Loader evidence

The [official configuration documentation](https://sing-box.sagernet.org/configuration/) describes JSON configuration and the check command. Exact directory selection is established by the official [1.14.1 loader](https://github.com/SagerNet/sing-box/blob/v1.14.1/cmd/sing-box/cmd_run.go#L68-L96), [1.14.4 loader](https://github.com/SagerNet/sing-box/blob/v1.14.4/cmd/sing-box/cmd_run.go#L68-L96), and [working-directory setup](https://github.com/SagerNet/sing-box/blob/v1.14.4/cmd/sing-box/cmd.go#L56-L68).

On November 6, 2026, isolated temporary fixtures were checked with the installed VPS binary (1.14.1) and local official binary (1.14.4). Both ignored invalid `.json.bak`, `.jsonc`, uppercase `.JSON`, and nested directory contents; both rejected invalid second and hidden `.json` files and followed `.json` symlinks. Relative `-C` paths resolved after `-D`. No live configuration was passed to the probe and no service was restarted.

## Validation and deployment safety

Candidates are written to a private temporary directory under Manager state. Directory-mode validation uses `check -C <candidate-directory> -D <effective-working-directory>`. Since only one source is writable, this substitutes the candidate for precisely that source while retaining relative resource resolution. No temporary active JSON file is placed beside production configuration. The directory is removed on success or failure.

Discovery refreshes before operations and after validation. A changed source path requires rediscovery, and added sources prevent deployment. Existing metadata-preserving atomic replacement, private history backups, transaction recovery, service readiness checks, and automatic rollback remain in place. There remains the existing external-administrator race between the final check and atomic replacement; administrators must not concurrently change the unit or its configuration sources during deployment.
