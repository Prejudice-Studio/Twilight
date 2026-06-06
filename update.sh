#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

systemctl stop twilight

git fetch origin main
git reset --hard origin/main

cd webui/
pnpm build
cd ..

go build -o bin/twilight ./cmd/twilight

for svc in twilight twilight-bot twilight-scheduler; do
  if systemctl is-enabled --quiet "$svc" 2>/dev/null; then
    systemctl restart "$svc"
  fi
done
