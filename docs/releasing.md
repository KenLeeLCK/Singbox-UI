# Publishing a release

No remote repository or release has been created by this implementation. Choose an authorized repository before running these commands; they are documentation, not evidence of publication.

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
git branch -M main
git remote add origin git@github.com:YOUR_ORG/YOUR_REPOSITORY.git
git add .
git commit -m "Implement sing-box Manager"
git push -u origin main
git tag v0.1.0
git push origin v0.1.0
```

The tag-triggered GitHub Actions workflow builds Linux amd64/arm64 archives, embeds the frontend, generates SHA256 checksums and publishes release assets. Inspect the workflow result and verify the release page/assets before distributing an installer command. CI definitions alone are not proof of a hosted successful release.

Release artifacts contain `manager`, `scripts/`, `deploy/`, `README.md` and `LICENSE`. The deployment scripts download only explicitly requested Manager release versions. sing-box installation is separately pinned to verified upstream 1.14.4 artifacts; Manager updates do not upgrade sing-box.

For future upstream upgrades, update protocol/schema tests, inspect official migration documentation, verify fresh release digests, update pinned dependencies and execute clean/existing/occupied-port/failure/rollback/interruption installer scenarios again. Keep dependency lockfiles and build toolchain constraints in the release commit.
