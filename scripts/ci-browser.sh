#!/usr/bin/env bash
# Linux CI integration, never installs/restarts host services.
set -euo pipefail
cd "$(dirname "$0")/.."
case $(uname -m) in x86_64|amd64) arch=amd64;; aarch64|arm64) arch=arm64;; *) echo 'Unsupported CI architecture' >&2; exit 1;; esac
python3 scripts/release.py singbox --arch "$arch" --directory .local/ci-sing-box
export SING_BOX_BIN="$PWD/.local/ci-sing-box/unpacked/sing-box-1.14.4-linux-$arch/sing-box"
export SING_BOX_TEST_BINARY="$SING_BOX_BIN"
(cd backend && go test -race ./... && go build -trimpath -o ../artifacts/manager ./cmd/manager)
npm --prefix tests/browser ci
npx --prefix tests/browser playwright install --with-deps chromium
npm --prefix tests/browser test
