# Publishing a release

The repository is [KenLeeLCK/Singbox-UI](https://github.com/KenLeeLCK/Singbox-UI). As of November 6, 2026, no GitHub Release has been published. The commands below are maintainer instructions: pushing a version tag triggers publication.

```sh
npm --prefix frontend ci
npm --prefix frontend run lint
npm --prefix frontend run typecheck
npm --prefix frontend test
npm --prefix frontend run build
(cd backend && go test -race ./... && go vet ./...)
python3 tests/installer_test.py
bash scripts/package.sh v0.1.0
```

Inspect the diff and secrets exclusions before committing. Confirm `.tools`, `.local`, binaries, `.env`, tokens and real configuration files remain ignored.

```sh
git remote -v
git push origin main
git tag v0.1.0
git push origin v0.1.0
```

The tag-triggered GitHub Actions workflow builds Linux amd64/arm64 archives, embeds the frontend, generates SHA256 checksums and publishes release assets. Inspect the workflow result and verify the release page/assets before distributing an installer command. CI definitions alone are not proof of a hosted successful release.

Release artifacts contain `manager`, `scripts/`, `deploy/`, `README.md` and `LICENSE`. The deployment scripts download only explicitly requested Manager release versions. sing-box installation is separately pinned to verified upstream 1.14.4 artifacts; Manager updates do not upgrade sing-box.

For future upstream upgrades, update protocol/schema tests, inspect official migration documentation, verify fresh release digests, update pinned dependencies and execute clean/existing/occupied-port/failure/rollback/interruption installer scenarios again. Keep dependency lockfiles and build toolchain constraints in the release commit.
