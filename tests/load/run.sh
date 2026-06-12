#!/usr/bin/env bash
# Run Tuck load tests.  Requires k6 (https://k6.io/docs/getting-started/installation/).
#
# Usage:
#   ./tests/load/run.sh smoke          # sanity check (1 min)
#   ./tests/load/run.sh load           # baseline (5 min, 50 VU)
#   ./tests/load/run.sh stress         # ramp to 200 VU (10 min)
#   ./tests/load/run.sh soak           # 24-hour stability run
#
# Environment:
#   TUCK_ADDR   — default https://127.0.0.1:8200
#   TUCK_TOKEN  — required (root or scoped token with read/write/token perms)

set -euo pipefail

SCENARIO="${1:-load}"
SCRIPT="$(dirname "$0")/k6.js"

: "${TUCK_ADDR:=https://127.0.0.1:8200}"
: "${TUCK_TOKEN:?TUCK_TOKEN must be set}"

echo "Tuck load test — scenario: $SCENARIO"
echo "Target: $TUCK_ADDR"
echo ""

k6 run \
  -e SCENARIO="$SCENARIO" \
  -e TUCK_ADDR="$TUCK_ADDR" \
  -e TUCK_TOKEN="$TUCK_TOKEN" \
  --insecure-skip-tls-verify \
  "$SCRIPT"
