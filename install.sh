#!/usr/bin/env sh
set -eu

NAME="diss"
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
COMMAND_PATH=$(command -v "$NAME" 2>/dev/null || true)

if [ -z "${INSTALL_DIR:-}" ]; then
  if [ -n "$COMMAND_PATH" ] && [ -f "$COMMAND_PATH" ]; then
    INSTALL_DIR=$(dirname "$COMMAND_PATH")
  else
    INSTALL_DIR="$HOME/.local/bin"
  fi
fi

mkdir -p "$INSTALL_DIR"
INSTALL_DIR=$(CDPATH= cd -- "$INSTALL_DIR" && pwd)
TARGET="$INSTALL_DIR/$NAME"
BUILD_ARTIFACT="$ROOT/.$NAME.build.$$"
TARGET_ARTIFACT="$INSTALL_DIR/.$NAME.install.$$"

cleanup() {
  rm -f "$BUILD_ARTIFACT" "$TARGET_ARTIFACT"
}
trap cleanup EXIT HUP INT TERM

if command -v go >/dev/null 2>&1; then
  printf 'Building %s from source with %s\n' "$NAME" "$(command -v go)"
  go build -o "$BUILD_ARTIFACT" "$ROOT"
  ARTIFACT="$BUILD_ARTIFACT"
else
  case "$(uname -s)-$(uname -m)" in
    Linux-x86_64) ARTIFACT="$ROOT/releases/$NAME-linux-amd64" ;;
    Linux-aarch64|Linux-arm64) ARTIFACT="$ROOT/releases/$NAME-linux-arm64" ;;
    Darwin-x86_64) ARTIFACT="$ROOT/releases/$NAME-darwin-amd64" ;;
    Darwin-arm64) ARTIFACT="$ROOT/releases/$NAME-darwin-arm64" ;;
    *) printf '%s\n' "Go is unavailable and no matching release artifact exists." >&2; exit 1 ;;
  esac
  if [ ! -f "$ARTIFACT" ]; then
    printf 'Matching release artifact not found: %s\n' "$ARTIFACT" >&2
    exit 1
  fi
  printf 'Go unavailable; installing release artifact %s\n' "$ARTIFACT"
fi

cp "$ARTIFACT" "$TARGET_ARTIFACT"
chmod 0755 "$TARGET_ARTIFACT"
mv -f "$TARGET_ARTIFACT" "$TARGET"

if [ ! -x "$TARGET" ]; then
  printf 'Installation failed; executable not found: %s\n' "$TARGET" >&2
  exit 1
fi
printf 'Installed %s to %s\n' "$NAME" "$TARGET"

RESOLVED_PATH=$(command -v "$NAME" 2>/dev/null || true)
if [ -z "$RESOLVED_PATH" ]; then
  printf 'Warning: %s is not currently in PATH; add %s to PATH.\n' "$TARGET" "$INSTALL_DIR"
elif [ "$RESOLVED_PATH" != "$TARGET" ]; then
  printf 'Warning: PATH resolves %s, not the newly installed %s.\n' "$RESOLVED_PATH" "$TARGET"
fi
