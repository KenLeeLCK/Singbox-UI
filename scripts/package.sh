#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
VERSION=${1:?Usage: package.sh vX.Y.Z}
[[ $VERSION =~ ^v[0-9]+\.[0-9]+\.[0-9]+([.-][A-Za-z0-9.-]+)?$ ]] || exit 2
mkdir -p artifacts
for arch in amd64 arm64; do
  stage=$(mktemp -d)
  (cd backend && CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -trimpath -o "$stage/manager" ./cmd/manager)
  mkdir "$stage/scripts"
  cp scripts/*.sh scripts/*.py "$stage/scripts/"
  cp -R deploy docs "$stage/"
  cp LICENSE README.md "$stage/"
  COPYFILE_DISABLE=1 tar -czf "artifacts/sing-box-manager-${VERSION}-linux-${arch}.tar.gz" -C "$stage" .
  rm -rf "$stage"
done
(cd artifacts && shasum -a 256 "sing-box-manager-${VERSION}-linux-"*.tar.gz > SHA256SUMS)
