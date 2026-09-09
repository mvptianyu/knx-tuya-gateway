#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
ENV_FILE="${PUBLISH_ENV_FILE:-$ROOT_DIR/.env.rpi}"

if [[ -f "$ENV_FILE" ]]; then
  set -a
  # shellcheck disable=SC1090
  source "$ENV_FILE"
  set +a
fi

SOURCE="${RUNTIME_BUNDLE_SOURCE:-$ROOT_DIR/data/knx-mapping-for-upgrade.json}"
TARGET_REL="${GITEE_BUNDLE_PATH:-knx/knx-mapping.json}"
REMOTE="${GITEE_REMOTE:-https://gitee.com/mvptianyu/config-gateway.git}"
BRANCH="${GITEE_BRANCH:-master}"
MESSAGE="${GITEE_COMMIT_MESSAGE:-chore: publish KNX runtime bundle}"
DRY_RUN="${GITEE_DRY_RUN:-0}"

PUBLISH_DIR=""
ASKPASS_FILE=""
cleanup() {
  [[ -z "$PUBLISH_DIR" ]] || rm -rf "$PUBLISH_DIR"
  [[ -z "$ASKPASS_FILE" ]] || rm -f "$ASKPASS_FILE"
}
trap cleanup EXIT

case "$REMOTE" in
  https://gitee.com/*)
    : "${GITEE_USERNAME:?Set GITEE_USERNAME in .env.rpi}"
    GITEE_SECRET="${GITEE_TOKEN:-${GITEE_PASSWORD:-}}"
    if [[ -z "$GITEE_SECRET" ]]; then
      echo "Set GITEE_TOKEN or GITEE_PASSWORD in .env.rpi" >&2
      exit 1
    fi
    ASKPASS_FILE="$(mktemp "${TMPDIR:-/tmp}/knx-gitee-askpass.XXXXXX")"
    printf '%s\n' \
      '#!/bin/sh' \
      'case "$1" in' \
      '  *Username*) printf "%s\n" "$KNX_GITEE_USERNAME" ;;' \
      '  *Password*) printf "%s\n" "$KNX_GITEE_SECRET" ;;' \
      '  *) exit 1 ;;' \
      'esac' > "$ASKPASS_FILE"
    chmod 0700 "$ASKPASS_FILE"
    export KNX_GITEE_USERNAME="$GITEE_USERNAME"
    export KNX_GITEE_SECRET="$GITEE_SECRET"
    export GIT_ASKPASS="$ASKPASS_FILE"
    export GIT_TERMINAL_PROMPT=0
    ;;
esac

case "$TARGET_REL" in
  /*|../*|*/../*)
    echo "GITEE_BUNDLE_PATH must stay inside the repository: $TARGET_REL" >&2
    exit 2
    ;;
esac

if [[ ! -f "$SOURCE" ]]; then
  echo "runtime bundle not found: $SOURCE" >&2
  echo "run: make mapping ACTION=finalize VERSION=YYYY.MM.DD-N" >&2
  exit 1
fi

GOCACHE="${GOCACHE:-/tmp/knx-tuya-go-cache}" \
  go run "$ROOT_DIR/cmd/knx-mapping-tool" validate \
  -input "$SOURCE" -config "$ROOT_DIR/config/config.json"

if git -C "$ROOT_DIR" remote get-url "$REMOTE" >/dev/null 2>&1; then
  REMOTE="$(git -C "$ROOT_DIR" remote get-url "$REMOTE")"
fi

PUBLISH_DIR="$(mktemp -d "${TMPDIR:-/tmp}/knx-config-gitee.XXXXXX")"
git -c http.lowSpeedLimit=1 -c http.lowSpeedTime=30 \
  clone --quiet --depth 1 --branch "$BRANCH" "$REMOTE" "$PUBLISH_DIR"
mkdir -p "$PUBLISH_DIR/$(dirname "$TARGET_REL")"
cp "$SOURCE" "$PUBLISH_DIR/$TARGET_REL"
git -C "$PUBLISH_DIR" add -- "$TARGET_REL"
if git -C "$PUBLISH_DIR" diff --cached --quiet -- "$TARGET_REL"; then
  echo "no runtime bundle change to publish"
  exit 0
fi

AUTHOR_NAME="${GITEE_GIT_AUTHOR_NAME:-$(git config user.name || true)}"
AUTHOR_EMAIL="${GITEE_GIT_AUTHOR_EMAIL:-$(git config user.email || true)}"
git -C "$PUBLISH_DIR" -c user.name="${AUTHOR_NAME:-knx-tuya-gateway}" \
  -c user.email="${AUTHOR_EMAIL:-knx-tuya-gateway@noreply.gitee.com}" \
  commit --quiet -m "$MESSAGE" -- "$TARGET_REL"
if [[ "$DRY_RUN" == "1" || "$DRY_RUN" == "true" ]]; then
  git -C "$PUBLISH_DIR" -c http.lowSpeedLimit=1 -c http.lowSpeedTime=30 \
    push --dry-run origin "HEAD:$BRANCH"
  echo "dry-run passed: Gitee authentication and push permission are valid"
  exit 0
fi
git -C "$PUBLISH_DIR" -c http.lowSpeedLimit=1 -c http.lowSpeedTime=30 \
  push origin "HEAD:$BRANCH"
echo "published: remote=$REMOTE branch=$BRANCH path=$TARGET_REL"
