#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

mkdir -p .gotmp
export GOTMPDIR="$PWD/.gotmp"
exec go run ./cmd/lingeyebot "$@"
