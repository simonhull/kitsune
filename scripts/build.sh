#!/bin/sh
# Build dist/kitsune-server with the web client inside it. The client is only
# in internal/webui/dist for the compile, so later plain builds stay API-only.
set -eu
cd "$(dirname "$0")/.."
clean() { find internal/webui/dist -mindepth 1 ! -name .gitkeep -delete; }
trap clean EXIT
clean
(cd web && bun install --frozen-lockfile && bun run build)
cp -R web/build/. internal/webui/dist/
CGO_ENABLED=0 go build -trimpath -o dist/kitsune-server ./cmd/kitsune-server
