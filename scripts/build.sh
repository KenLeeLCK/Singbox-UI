#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
npm --prefix frontend ci
npm --prefix frontend run build
mkdir -p artifacts
(cd backend && CGO_ENABLED=0 go build -trimpath -o ../artifacts/manager ./cmd/manager)
