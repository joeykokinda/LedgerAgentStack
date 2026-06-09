#!/usr/bin/env bash
# One command: Speculos + airlock + agent demo, end to end, capturing proof.
#
#   ./scripts/demo.sh
#
# Starts the Ledger device emulator, builds and launches the airlock with the
# Speculos signer wired in, runs the three-scenario agent demo against it, and
# leaves the captured screenshots and logs in ./proof.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

AIRLOCK_PORT="${AIRLOCK_PORT:-18080}"
BASE="http://localhost:${AIRLOCK_PORT}"
# Keep the Speculos API port consistent between the emulator and the signer
# (the airlock passes this env through to node signer/speculos-sign.js).
export SPECULOS_API_PORT="${SPECULOS_API_PORT:-5000}"

echo ">> starting Speculos (Ethereum app, headless) on API :${SPECULOS_API_PORT}"
API_HOST_PORT="$SPECULOS_API_PORT" ./scripts/run-speculos.sh

echo ">> building airlock + agent-demo"
go build -o bin/airlock ./cmd/airlock
go build -o bin/agent-demo ./cmd/agent-demo

echo ">> launching airlock on :${AIRLOCK_PORT} (signer: node signer/speculos-sign.js)"
rm -f proof/audit.ndjson
./bin/airlock \
  --policy policy/example-policy.yaml \
  --signer-cmd "node signer/speculos-sign.js" \
  --audit-log proof/audit.ndjson \
  --addr ":${AIRLOCK_PORT}" &
AIRLOCK_PID=$!
trap 'kill "$AIRLOCK_PID" 2>/dev/null || true' EXIT

echo ">> waiting for airlock health"
curl -fsS --retry 60 --retry-delay 1 --retry-all-errors --retry-max-time 70 -m 5 "${BASE}/healthz" >/dev/null

echo ">> running agent demo (signs scenario 1 + 3-primer on the device, blocks 2 + 3-drain)"
./bin/agent-demo --addr "$BASE" | tee proof/agent-demo-run.txt

echo
echo ">> done. proof artifacts:"
ls -1 proof/
echo ">> Speculos is still running. Stop everything with ./scripts/stop.sh"
