#!/usr/bin/env bash
# Launch Speculos (Ledger device emulator) running the Ethereum app.
# Validated on Nano X with app-ethereum 1.22.1. No physical device required.
#
# Ports:
#   API  (screenshot / buttons / events)  host 5111 -> container 5000
#   APDU (signing transport)              host 9999 -> container 9999
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ELF="${ELF:-$ROOT/speculos-apps/eth-nanox.elf}"
MODEL="${MODEL:-nanox}"
API_HOST_PORT="${API_HOST_PORT:-5000}"
APDU_HOST_PORT="${APDU_HOST_PORT:-9999}"
NAME="${NAME:-speculos-eth}"
IMAGE="ghcr.io/ledgerhq/speculos:latest"

if [ ! -f "$ELF" ]; then
  echo "ETH app ELF not found at $ELF"
  echo "Fetching app-ethereum 1.22.1 ($MODEL) from GitHub releases..."
  mkdir -p "$(dirname "$ELF")"
  curl -fL --retry 3 -o "$ELF" \
    "https://github.com/LedgerHQ/app-ethereum/releases/download/1.22.1/app-1.22.1-${MODEL}.elf"
fi

docker rm -f "$NAME" >/dev/null 2>&1 || true

docker run --rm -d --name "$NAME" \
  -v "$(dirname "$ELF")":/apps \
  -p "${API_HOST_PORT}:5000" \
  -p "${APDU_HOST_PORT}:9999" \
  "$IMAGE" \
  --model "$MODEL" --display headless --api-port 5000 "/apps/$(basename "$ELF")" >/dev/null

# Wait for the HTTP API to answer (retry through connection resets, no sleep).
if curl -fsS --retry 60 --retry-delay 1 --retry-all-errors --retry-max-time 75 -m 8 \
     "http://localhost:${API_HOST_PORT}/screenshot" -o /dev/null; then
  echo "Speculos up: API http://localhost:${API_HOST_PORT}  APDU localhost:${APDU_HOST_PORT}  app=Ethereum/${MODEL}"
else
  echo "Speculos API did not come up; logs:" >&2
  docker logs "$NAME" 2>&1 | tail -30 >&2
  exit 1
fi
