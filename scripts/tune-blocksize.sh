#!/usr/bin/env bash
# sweeps step 2's block size and reports which one yields the longest key
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."

exec go run ./cmd/tuneblock "$@"
