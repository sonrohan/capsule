#!/bin/sh
set -eu

REPO="sonrohan/capsule"
BINARY="capsule"
ARCHIVE="capsule_Darwin_all.tar.gz"
INSTALL_DIR="${CAPSULE_INSTALL_DIR:-$HOME/.local/bin}"
TMP_DIR="$(mktemp -d)"

cleanup() {
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT INT TERM

fail() {
  echo "capsule install: $*" >&2
  exit 1
}

command_exists() {
  command -v "$1" >/dev/null 2>&1
}

if [ "$(uname -s)" != "Darwin" ]; then
  fail "this installer currently supports macOS only. Linux and Windows support are welcome via PR."
fi

if [ -z "${VERSION:-}" ]; then
  if command_exists curl; then
    VERSION="$(curl -fsSL -o /dev/null -w '%{url_effective}' "https://github.com/$REPO/releases/latest" | sed 's#.*/##')"
  else
    fail "curl is required"
  fi
fi

case "$VERSION" in
  v*) ;;
  *) VERSION="v$VERSION" ;;
esac

BASE_URL="https://github.com/$REPO/releases/download/$VERSION"
ARCHIVE_URL="$BASE_URL/$ARCHIVE"
CHECKSUMS_URL="$BASE_URL/checksums.txt"

echo "Installing capsule $VERSION for macOS..."

curl -fsSL "$ARCHIVE_URL" -o "$TMP_DIR/$ARCHIVE"
curl -fsSL "$CHECKSUMS_URL" -o "$TMP_DIR/checksums.txt"

EXPECTED="$(awk -v archive="$ARCHIVE" '$2 == archive { print $1 }' "$TMP_DIR/checksums.txt")"
if [ -z "$EXPECTED" ]; then
  fail "could not find $ARCHIVE in checksums.txt"
fi

if command_exists shasum; then
  ACTUAL="$(shasum -a 256 "$TMP_DIR/$ARCHIVE" | awk '{print $1}')"
elif command_exists sha256sum; then
  ACTUAL="$(sha256sum "$TMP_DIR/$ARCHIVE" | awk '{print $1}')"
else
  fail "shasum or sha256sum is required to verify the download"
fi

if [ "$EXPECTED" != "$ACTUAL" ]; then
  fail "checksum mismatch for $ARCHIVE"
fi

tar -xzf "$TMP_DIR/$ARCHIVE" -C "$TMP_DIR"

if [ ! -f "$TMP_DIR/$BINARY" ]; then
  fail "archive did not contain $BINARY"
fi

mkdir -p "$INSTALL_DIR"
install -m 0755 "$TMP_DIR/$BINARY" "$INSTALL_DIR/$BINARY"

echo "Installed capsule to $INSTALL_DIR/$BINARY"

case ":$PATH:" in
  *":$INSTALL_DIR:"*)
    echo "Run: capsule --help"
    ;;
  *)
    echo "Add $INSTALL_DIR to PATH to run capsule from anywhere:"
    echo "  echo 'export PATH=\"$INSTALL_DIR:\$PATH\"' >> ~/.zshrc"
    echo "Then reload your shell:"
    echo "  exec zsh"
    ;;
esac
