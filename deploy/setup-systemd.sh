#!/usr/bin/env bash
set -euo pipefail

SERVICE_NAMES=("twilight" "twilight-bot" "twilight-scheduler")
LEGACY_PATTERN='python|uvicorn|gunicorn|main\.py|asgi\.py|src\.'

usage() {
  cat <<'EOF'
Usage:
  sudo bash deploy/setup-systemd.sh [--dry-run] [--no-build] [--restart]

Environment overrides:
  TWILIGHT_PROJECT_ROOT       Project root. Defaults to the parent of deploy/.
  TWILIGHT_GO_BIN            Backend binary path. Defaults to <project>/bin/twilight.
  TWILIGHT_CONFIG_FILE        Config path. Defaults to <project>/config.toml.
  TWILIGHT_API_HOST           API bind host. Defaults to 127.0.0.1.
  TWILIGHT_API_PORT           API bind port. Defaults to 5000.
  TWILIGHT_SYSTEMD_USER       systemd service user. Defaults to root.
  TWILIGHT_SYSTEMD_GROUP      systemd service group. Defaults to TWILIGHT_SYSTEMD_USER.
EOF
}

DRY_RUN=0
NO_BUILD=0
RESTART=0
for arg in "$@"; do
  case "$arg" in
    --dry-run) DRY_RUN=1 ;;
    --no-build) NO_BUILD=1 ;;
    --restart) RESTART=1 ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown argument: $arg" >&2
      usage >&2
      exit 2
      ;;
  esac
done

if [[ "$(uname -s)" != "Linux" ]]; then
  echo "This setup script is Linux-only." >&2
  exit 1
fi

if [[ "${EUID:-$(id -u)}" -ne 0 ]]; then
  echo "Run as root: sudo bash deploy/setup-systemd.sh" >&2
  exit 1
fi

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "Missing required command: $1" >&2
    exit 1
  fi
}

need_cmd systemctl
need_cmd realpath
need_cmd install
need_cmd mktemp

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(realpath "${TWILIGHT_PROJECT_ROOT:-"$SCRIPT_DIR/.."}")"
BIN_PATH="$(realpath -m "${TWILIGHT_GO_BIN:-"$PROJECT_ROOT/bin/twilight"}")"
CONFIG_FILE="$(realpath -m "${TWILIGHT_CONFIG_FILE:-"$PROJECT_ROOT/config.toml"}")"
ENV_FILE="$PROJECT_ROOT/.env"
API_HOST="${TWILIGHT_API_HOST:-127.0.0.1}"
API_PORT="${TWILIGHT_API_PORT:-5000}"
SERVICE_USER="${TWILIGHT_SYSTEMD_USER:-root}"
SERVICE_GROUP="${TWILIGHT_SYSTEMD_GROUP:-$SERVICE_USER}"
UNIT_DIR="/etc/systemd/system"
TMP_CHECK="$(mktemp -t twilight-systemd-check.XXXXXX)"
cleanup() {
  rm -f "$TMP_CHECK"
}
trap cleanup EXIT

if [[ ! -d "$PROJECT_ROOT" ]]; then
  echo "Project root does not exist: $PROJECT_ROOT" >&2
  exit 1
fi

if [[ ! -f "$PROJECT_ROOT/go.mod" || ! -d "$PROJECT_ROOT/cmd/twilight" ]]; then
  echo "Project root does not look like Twilight Go backend: $PROJECT_ROOT" >&2
  exit 1
fi

if [[ "$PROJECT_ROOT$BIN_PATH$CONFIG_FILE$ENV_FILE" =~ [[:space:]] ]]; then
  echo "systemd setup does not support whitespace in project, binary, config, or env paths." >&2
  exit 1
fi
if [[ "$PROJECT_ROOT$BIN_PATH$CONFIG_FILE$ENV_FILE" == *%* ]]; then
  echo "systemd setup does not support '%' in project, binary, config, or env paths because systemd treats it as a specifier." >&2
  exit 1
fi
if [[ "$API_HOST$API_PORT$SERVICE_USER$SERVICE_GROUP" =~ [[:space:]] ]]; then
  echo "systemd setup does not support whitespace in host, port, user, or group values." >&2
  exit 1
fi
if ! [[ "$API_PORT" =~ ^[0-9]{1,5}$ ]] || (( API_PORT < 1 || API_PORT > 65535 )); then
  echo "Invalid API port: $API_PORT" >&2
  exit 1
fi

if ! id "$SERVICE_USER" >/dev/null 2>&1; then
  echo "Service user does not exist: $SERVICE_USER" >&2
  exit 1
fi
if ! getent group "$SERVICE_GROUP" >/dev/null 2>&1; then
  echo "Service group does not exist: $SERVICE_GROUP" >&2
  exit 1
fi

CONFIG_WILL_CREATE=0
if [[ ! -f "$CONFIG_FILE" ]]; then
  if [[ -f "$PROJECT_ROOT/config.production.toml" ]]; then
    echo "Config not found; will create from config.production.toml: $CONFIG_FILE"
    CONFIG_WILL_CREATE=1
    if [[ "$DRY_RUN" -eq 0 ]]; then
      install -d -o "$SERVICE_USER" -g "$SERVICE_GROUP" -m 0750 "$(dirname "$CONFIG_FILE")"
      install -o "$SERVICE_USER" -g "$SERVICE_GROUP" -m 0640 "$PROJECT_ROOT/config.production.toml" "$CONFIG_FILE"
      CONFIG_WILL_CREATE=0
    fi
  else
    echo "Config file not found: $CONFIG_FILE" >&2
    exit 1
  fi
fi

if [[ ! -x "$BIN_PATH" ]]; then
  if [[ "$NO_BUILD" -eq 1 ]]; then
    echo "Backend binary is missing or not executable: $BIN_PATH" >&2
    exit 1
  fi
  need_cmd go
  echo "Building backend binary: $BIN_PATH"
  if [[ "$DRY_RUN" -eq 0 ]]; then
    install -d -m 0755 "$(dirname "$BIN_PATH")"
    (cd "$PROJECT_ROOT" && go build -o "$BIN_PATH" ./cmd/twilight)
    chmod 0755 "$BIN_PATH"
  fi
fi

if [[ "$DRY_RUN" -eq 0 ]]; then
  install -d -o "$SERVICE_USER" -g "$SERVICE_GROUP" -m 0750 \
    "$PROJECT_ROOT/db" \
    "$PROJECT_ROOT/db/backups" \
    "$PROJECT_ROOT/uploads" \
    "$PROJECT_ROOT/config_backups"
fi

if [[ "$CONFIG_WILL_CREATE" -eq 0 ]] && command -v runuser >/dev/null 2>&1; then
  if ! runuser -u "$SERVICE_USER" -- test -r "$CONFIG_FILE"; then
    echo "Config is not readable by service user $SERVICE_USER: $CONFIG_FILE" >&2
    exit 1
  fi
  if [[ -f "$ENV_FILE" ]] && ! runuser -u "$SERVICE_USER" -- test -r "$ENV_FILE"; then
    echo "Warning: env file exists but is not readable by service user $SERVICE_USER: $ENV_FILE" >&2
  fi
fi

unit_paths_for() {
  local name="$1"
  printf '%s\n' \
    "$UNIT_DIR/$name.service" \
    "/lib/systemd/system/$name.service" \
    "/usr/lib/systemd/system/$name.service"
}

legacy_units=()
for name in "${SERVICE_NAMES[@]}"; do
  while IFS= read -r unit_path; do
    [[ -f "$unit_path" ]] || continue
    if grep -Eiq "$LEGACY_PATTERN" "$unit_path"; then
      legacy_units+=("$unit_path")
    fi
  done < <(unit_paths_for "$name")

  if systemctl cat "$name.service" >"$TMP_CHECK" 2>/dev/null; then
    if grep -Eiq "$LEGACY_PATTERN" "$TMP_CHECK"; then
      legacy_units+=("systemctl:$name.service")
    fi
  fi
done

if [[ "${#legacy_units[@]}" -gt 0 ]]; then
  echo "Detected legacy Python Twilight systemd units:"
  printf '  - %s\n' "${legacy_units[@]}"
  if [[ "$DRY_RUN" -eq 0 ]]; then
    stamp="$(date +%Y%m%d%H%M%S)"
    for name in "${SERVICE_NAMES[@]}"; do
      systemctl stop "$name.service" >/dev/null 2>&1 || true
      systemctl disable "$name.service" >/dev/null 2>&1 || true
    done
    for item in "${legacy_units[@]}"; do
      [[ "$item" == systemctl:* ]] && continue
      cp -a "$item" "$item.python-legacy.$stamp.bak"
      echo "Backed up legacy unit: $item.python-legacy.$stamp.bak"
    done
  fi
fi

print_summary() {
  cat <<EOF
Twilight systemd setup
  project_root: $PROJECT_ROOT
  binary:       $BIN_PATH
  config:       $CONFIG_FILE
  env_file:     $ENV_FILE
  api:          $API_HOST:$API_PORT
  user/group:   $SERVICE_USER:$SERVICE_GROUP
  unit_dir:     $UNIT_DIR
EOF
}

write_api_unit() {
  cat >"$UNIT_DIR/twilight.service" <<EOF
[Unit]
Description=Twilight Go API
After=network-online.target
Wants=network-online.target

[Service]
Type=exec
User=$SERVICE_USER
Group=$SERVICE_GROUP
WorkingDirectory=$PROJECT_ROOT
ExecStart=$BIN_PATH api --host $API_HOST --port $API_PORT --config $CONFIG_FILE

Environment=TWILIGHT_CONFIG_FILE=$CONFIG_FILE
EnvironmentFile=-$ENV_FILE

LimitNOFILE=65535
LimitNPROC=4096
MemoryMax=1G
MemoryHigh=768M

Restart=always
RestartSec=5
StartLimitIntervalSec=60
StartLimitBurst=5

TimeoutStopSec=30
KillMode=mixed
KillSignal=SIGTERM

StandardOutput=journal
StandardError=journal
SyslogIdentifier=twilight

[Install]
WantedBy=multi-user.target
EOF
}

write_worker_unit() {
  local name="$1"
  local description="$2"
  local command="$3"
  local memory_max="$4"
  local timeout="$5"
  cat >"$UNIT_DIR/$name.service" <<EOF
[Unit]
Description=$description
After=network-online.target twilight.service
Wants=network-online.target
PartOf=twilight.service

[Service]
Type=exec
User=$SERVICE_USER
Group=$SERVICE_GROUP
WorkingDirectory=$PROJECT_ROOT
ExecStart=$BIN_PATH $command --config $CONFIG_FILE

Environment=TWILIGHT_CONFIG_FILE=$CONFIG_FILE
EnvironmentFile=-$ENV_FILE

LimitNOFILE=65535
MemoryMax=$memory_max
MemoryHigh=384M

Restart=always
RestartSec=10
StartLimitIntervalSec=300
StartLimitBurst=5

TimeoutStopSec=$timeout
KillMode=mixed
KillSignal=SIGTERM

StandardOutput=journal
StandardError=journal
SyslogIdentifier=$name

[Install]
WantedBy=multi-user.target
EOF
}

print_summary

if [[ "$DRY_RUN" -eq 1 ]]; then
  echo "Dry run only; no systemd units were written."
  exit 0
fi

write_api_unit
write_worker_unit "twilight-bot" "Twilight Go Telegram Bot Bridge" "bot" "512M" "15"
write_worker_unit "twilight-scheduler" "Twilight Go Scheduler" "scheduler" "512M" "30"

systemctl daemon-reload
systemctl enable twilight.service twilight-bot.service twilight-scheduler.service

if [[ "$RESTART" -eq 1 ]]; then
  systemctl restart twilight.service twilight-bot.service twilight-scheduler.service
else
  systemctl start twilight.service twilight-bot.service twilight-scheduler.service
fi

systemctl --no-pager --full status twilight.service twilight-bot.service twilight-scheduler.service || true
