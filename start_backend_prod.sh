#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

MODE="${1:-api}"
shift 2>/dev/null || true

HOST="${TWILIGHT_API_HOST:-0.0.0.0}"
PORT="${TWILIGHT_API_PORT:-5000}"
CONFIG="config.toml"
NOFILE="${TWILIGHT_NOFILE_LIMIT:-65535}"

echo "=========================================="
echo "   Twilight Backend (Go Production)"
echo "=========================================="
echo "Mode: $MODE  Host: $HOST  Port: $PORT"
echo "Config: $CONFIG"

if command -v ulimit >/dev/null 2>&1; then
  CURRENT_NOFILE="$(ulimit -n || true)"
  if ulimit -n "$NOFILE" 2>/dev/null; then
    echo "NOFILE: $(ulimit -n)"
  else
    echo "WARNING: unable to raise NOFILE to $NOFILE (current: $CURRENT_NOFILE). Configure systemd LimitNOFILE or container ulimit."
  fi
fi

if [[ -n "${TWILIGHT_GO_BIN:-}" ]]; then
  exec "$TWILIGHT_GO_BIN" "$MODE" --host "$HOST" --port "$PORT" --config "$CONFIG" "$@"
fi

if [[ -x "./bin/twilight" ]]; then
  exec ./bin/twilight "$MODE" --host "$HOST" --port "$PORT" --config "$CONFIG" "$@"
fi

echo "WARNING: ./bin/twilight not found; falling back to go run. Build with: go build -o bin/twilight ./cmd/twilight"
exec go run ./cmd/twilight "$MODE" --host "$HOST" --port "$PORT" --config "$CONFIG" "$@"
