#!/usr/bin/env sh
set -eu

NAME="diss"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)

if command -v go >/dev/null 2>&1; then
  cd "$ROOT"
  go build -o "$NAME" .
  ARTIFACT="$ROOT/$NAME"
else
  case "$(uname -s)-$(uname -m)" in
    Linux-x86_64) ARTIFACT="$ROOT/releases/$NAME-linux-amd64" ;;
    Linux-aarch64|Linux-arm64) ARTIFACT="$ROOT/releases/$NAME-linux-arm64" ;;
    Darwin-x86_64) ARTIFACT="$ROOT/releases/$NAME-darwin-amd64" ;;
    Darwin-arm64) ARTIFACT="$ROOT/releases/$NAME-darwin-arm64" ;;
    *) printf '%s\n' "Go is unavailable and no matching release artifact exists." >&2; exit 1 ;;
  esac
fi

mkdir -p "$INSTALL_DIR"
cp "$ARTIFACT" "$INSTALL_DIR/$NAME"
chmod +x "$INSTALL_DIR/$NAME"
printf 'Installed %s to %s\n' "$NAME" "$INSTALL_DIR/$NAME"
