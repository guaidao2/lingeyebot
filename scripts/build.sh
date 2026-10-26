#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

# Build the web UI and embed it, then cross-compile a CGO-free binary.
if [ -f web/package.json ]; then
  if [ ! -d web/node_modules ]; then
    (cd web && npm install)
  fi
  (cd web && npm run build)
  rm -rf internal/server/dist
  cp -r web/dist internal/server/dist
fi

mkdir -p .gotmp bin
export GOTMPDIR="$PWD/.gotmp"
export CGO_ENABLED=0

GOOS="${GOOS:-linux}"
GOARCH="${GOARCH:-amd64}"
OUT="bin/lingeyebot-${GOOS}-${GOARCH}"
[ "$GOOS" = "windows" ] && OUT="${OUT}.exe"

GOOS="$GOOS" GOARCH="$GOARCH" go build -trimpath -ldflags "-s -w" -o "$OUT" ./cmd/lingeyebot
echo "built $OUT"
