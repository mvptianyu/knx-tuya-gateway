#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
SOURCE="${RUNTIME_BUNDLE_SOURCE:-$ROOT_DIR/data/knx-mapping-for-upgrade.json}"
TARGET_REL="${GITHUB_BUNDLE_PATH:-knx/knx-mapping.json}"
REMOTE="${GITHUB_REMOTE:-https://github.com/mvptianyu/config-gateway.git}"
BRANCH="${GITHUB_BRANCH:-main}"
MESSAGE="${GITHUB_COMMIT_MESSAGE:-chore: publish KNX runtime bundle}"

case "$TARGET_REL" in
  /*|../*|*/../*)
    echo "GITHUB_BUNDLE_PATH must stay inside the repository: $TARGET_REL" >&2
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

PUBLISH_DIR="$(mktemp -d "${TMPDIR:-/tmp}/knx-config-github.XXXXXX")"
trap 'rm -rf "$PUBLISH_DIR"' EXIT
git clone --quiet --depth 1 --branch "$BRANCH" "$REMOTE" "$PUBLISH_DIR"
mkdir -p "$PUBLISH_DIR/$(dirname "$TARGET_REL")"
cp "$SOURCE" "$PUBLISH_DIR/$TARGET_REL"
git -C "$PUBLISH_DIR" add -- "$TARGET_REL"
if git -C "$PUBLISH_DIR" diff --cached --quiet -- "$TARGET_REL"; then
  echo "no runtime bundle change to publish"
  exit 0
fi

AUTHOR_NAME="${GITHUB_GIT_AUTHOR_NAME:-$(git config user.name || true)}"
AUTHOR_EMAIL="${GITHUB_GIT_AUTHOR_EMAIL:-$(git config user.email || true)}"
git -C "$PUBLISH_DIR" -c user.name="${AUTHOR_NAME:-knx-tuya-gateway}" \
  -c user.email="${AUTHOR_EMAIL:-knx-tuya-gateway@users.noreply.github.com}" \
  commit --quiet -m "$MESSAGE" -- "$TARGET_REL"
git -C "$PUBLISH_DIR" push origin "HEAD:$BRANCH"
echo "published: remote=$REMOTE branch=$BRANCH path=$TARGET_REL"
