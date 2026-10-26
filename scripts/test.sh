#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

# Build and run test binaries inside the workspace so antivirus scanners do not
# quarantine executables created under the system temp directory.
mkdir -p .gotmp
export GOTMPDIR="$PWD/.gotmp"

go vet ./...
go test "$@" ./...
