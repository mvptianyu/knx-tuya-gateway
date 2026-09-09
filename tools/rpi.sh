#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
ENV_FILE="${RPI_ENV_FILE:-$ROOT_DIR/.env.rpi}"
ACTION="${1:-deploy}"

if [[ -f "$ENV_FILE" ]]; then
  set -a
  # shellcheck disable=SC1090
  source "$ENV_FILE"
  set +a
fi

build_arm64() {
  export GOTOOLCHAIN="${GOTOOLCHAIN:-go1.23.7}"
  export GOCACHE="${GOCACHE:-/tmp/knx-tuya-go-cache}"
  export GOOS=linux GOARCH=arm64 CGO_ENABLED=0
  mkdir -p "$ROOT_DIR/bin"
  go build -trimpath -ldflags="-s -w" -o "$ROOT_DIR/bin/knx-tuya-gw-arm64" "$ROOT_DIR"
  echo "built: bin/knx-tuya-gw-arm64"
}

collect_info() {
  printf '%s\n' "os-release:"
  cat /etc/os-release
  printf '%s\n' "" "kernel:"
  uname -a
  printf '%s\n' "" "architecture:"
  uname -m
  getconf LONG_BIT
  printf '%s\n' "" "hardware model:"
  tr -d '\0' < /proc/device-tree/model 2>/dev/null || true
  printf '%s\n' "" "" "network:"
  ip -brief address
}

if [[ "$ACTION" == "build" ]]; then
  build_arm64
  exit 0
fi
: "${RPI_HOST:?Set RPI_HOST in .env.rpi or the environment}"
RPI_USER="${RPI_USER:-pi}"
RPI_PORT="${RPI_PORT:-22}"
RPI_APP_DIR="${RPI_APP_DIR:-/opt/knx-tuya-gw}"
RPI_SUDO_PASSWORD="${RPI_SUDO_PASSWORD:-${RPI_PASSWORD:-}}"
TARGET="${RPI_USER}@${RPI_HOST}"
REMOTE_STAGE="/tmp/knx-tuya-gw-deploy"

SSH_BASE=(ssh -o StrictHostKeyChecking=accept-new -p "$RPI_PORT")
if [[ -n "${RPI_SSH_KEY:-}" ]]; then
  SSH_BASE+=(-i "$RPI_SSH_KEY")
fi
if [[ -n "${RPI_PASSWORD:-}" ]]; then
  ASKPASS_FILE="$(mktemp "${TMPDIR:-/tmp}/knx-rpi-askpass.XXXXXX")"
  trap 'rm -f "$ASKPASS_FILE"' EXIT
  printf '%s\n' '#!/bin/sh' 'printf "%s\n" "$KNX_RPI_SSH_PASSWORD"' > "$ASKPASS_FILE"
  chmod 0700 "$ASKPASS_FILE"
  export KNX_RPI_SSH_PASSWORD="$RPI_PASSWORD"
  export SSH_ASKPASS="$ASKPASS_FILE"
  export SSH_ASKPASS_REQUIRE=force
  export DISPLAY="${DISPLAY:-knx-rpi:0}"
  SSH=("${SSH_BASE[@]}" "$TARGET")
  RSYNC_RSH="ssh -o StrictHostKeyChecking=accept-new -p $RPI_PORT"
else
  SSH=("${SSH_BASE[@]}" "$TARGET")
  RSYNC_RSH="ssh -o StrictHostKeyChecking=accept-new -p $RPI_PORT"
fi
if [[ -n "${RPI_SSH_KEY:-}" ]]; then
  printf -v key_quoted '%q' "$RPI_SSH_KEY"
  RSYNC_RSH+=" -i $key_quoted"
fi

remote() {
  "${SSH[@]}" "$1"
}

remote_root() {
  local command="$1"
  local quoted
  printf -v quoted '%q' "$command"
  if [[ -n "$RPI_SUDO_PASSWORD" ]]; then
    printf '%s\n' "$RPI_SUDO_PASSWORD" |
      "${SSH[@]}" "sudo -S -p '' bash -lc $quoted"
  else
    "${SSH[@]}" "sudo -n bash -lc $quoted"
  fi
}

sync_files() {
  rsync -az -e "$RSYNC_RSH" "$@"
}

runtime_config=()
for config_file in config.json knx-mapping.json tuya-mqtt.env; do
  if [[ -f "$ROOT_DIR/config/$config_file" ]]; then
    runtime_config+=("$ROOT_DIR/config/$config_file")
  fi
done
if [[ -f "$ROOT_DIR/config/config.json" ]]; then
  ca_file="$(awk -F'"' '/"ca_file"[[:space:]]*:/ { print $4; exit }' "$ROOT_DIR/config/config.json")"
  if [[ -n "$ca_file" ]]; then
    if [[ "$ca_file" = /* || "$ca_file" == *"/"* || ! -f "$ROOT_DIR/config/$ca_file" ]]; then
      echo "tuya_mqtt.ca_file must name an existing file inside config/: $ca_file" >&2
      exit 1
    fi
    runtime_config+=("$ROOT_DIR/config/$ca_file")
  fi
fi

case "$ACTION" in
  deploy)
    build_arm64
    remote "rm -rf '$REMOTE_STAGE' && mkdir -p '$REMOTE_STAGE/config'"
    sync_files "$ROOT_DIR/bin" "$ROOT_DIR/deploy" "$TARGET:$REMOTE_STAGE/"
    if (( ${#runtime_config[@]} > 0 )); then
      sync_files "${runtime_config[@]}" "$TARGET:$REMOTE_STAGE/config/"
    fi
    remote_root "RPI_APP_DIR='$RPI_APP_DIR' bash '$REMOTE_STAGE/deploy/install.sh'"
    remote_root "systemctl restart knx-tuya-bridge.service && systemctl --no-pager --full status knx-tuya-bridge.service"
    ;;
  config)
    if (( ${#runtime_config[@]} == 0 )); then
      echo "no runtime files found in config/" >&2
      exit 1
    fi
    remote "rm -rf '$REMOTE_STAGE/config' && mkdir -p '$REMOTE_STAGE/config'"
    sync_files "${runtime_config[@]}" "$TARGET:$REMOTE_STAGE/config/"
    remote_root "install -d -m 0700 '$RPI_APP_DIR/config'; find '$REMOTE_STAGE/config' -maxdepth 1 -type f -exec install -m 0600 -t '$RPI_APP_DIR/config' {} +; systemctl restart knx-tuya-bridge.service"
    remote_root "systemctl --no-pager --full status knx-tuya-bridge.service"
    ;;
  status)
    remote_root "systemctl --no-pager --full status knx-tuya-bridge.service"
    ;;
  logs)
    remote_root "journalctl -u knx-tuya-bridge.service -n 100 -f"
    ;;
  restart)
    remote_root "systemctl restart knx-tuya-bridge.service && systemctl --no-pager --full status knx-tuya-bridge.service"
    ;;
  info)
    remote "$(declare -f collect_info); collect_info"
    ;;
  review)
    mkdir -p "$ROOT_DIR/data"
    sync_files \
      "$TARGET:$RPI_APP_DIR/state/knx-commission-review.csv" \
      "$ROOT_DIR/data/knx-commission-review.csv"
    echo "downloaded: data/knx-commission-review.csv"
    ;;
  *)
    echo "usage: $0 {build|deploy|config|status|logs|restart|info|review}" >&2
    exit 2
    ;;
esac
