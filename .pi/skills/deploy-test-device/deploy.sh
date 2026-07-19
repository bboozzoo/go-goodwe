#!/usr/bin/env bash
set -euo pipefail

HOST="${GOODWE_TEST_HOST:-}"
if [ -z "$HOST" ]; then
  echo "ERROR: GOODWE_TEST_HOST is not set." >&2
  echo "  Set it to the SSH hostname of your test Pi, e.g.:" >&2
  echo "    export GOODWE_TEST_HOST=some-pi.tailnet.ts.net" >&2
  exit 1
fi
BINARY="goodwe-daemon-arm"

echo "==> Building $BINARY for ARMv8..."
cd "$(dirname "$0")/../../.."
GOARCH=arm GOARM=8 go build -o "$BINARY" ./cmd/goodwe-daemon

echo "==> Deploying to $HOST..."
tar -cj "$BINARY" | ssh "$HOST" 'tar xjvf -'

echo "==> Restarting goodwe service..."
ssh "$HOST" 'sudo systemctl restart goodwe'

echo "==> Done."
