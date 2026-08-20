#!/bin/sh
# Install the Àgbàlá host client.
#
#   curl -fsSL https://raw.githubusercontent.com/tolaniverse/agbala/main/scripts/install.sh | sh
#
# Override the version with AGBALA_VERSION and the destination with AGBALA_BIN_DIR.
set -eu

REPO="tolaniverse/agbala"
BIN="agbala"
VERSION="${AGBALA_VERSION:-latest}"
BIN_DIR="${AGBALA_BIN_DIR:-}"

die() { echo "install: $*" >&2; exit 1; }
need() { command -v "$1" >/dev/null 2>&1 || die "$1 is required but not installed"; }

need uname
need tar
if command -v curl >/dev/null 2>&1; then
  fetch() { curl -fsSL "$1"; }
  fetch_to() { curl -fsSL "$1" -o "$2"; }
elif command -v wget >/dev/null 2>&1; then
  fetch() { wget -qO- "$1"; }
  fetch_to() { wget -qO "$2" "$1"; }
else
  die "either curl or wget is required"
fi

os=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$os" in
  darwin|linux) ;;
  *) die "unsupported operating system: $os" ;;
esac

arch=$(uname -m)
case "$arch" in
  x86_64|amd64) arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *) die "unsupported architecture: $arch" ;;
esac

if [ "$VERSION" = latest ]; then
  VERSION=$(fetch "https://api.github.com/repos/$REPO/releases/latest" \
    | sed -n 's/.*"tag_name" *: *"\([^"]*\)".*/\1/p' | head -n1)
  [ -n "$VERSION" ] || die "could not determine the latest release"
fi
strip_v=${VERSION#v}

# Pick a destination we can actually write to, preferring one already on PATH.
if [ -z "$BIN_DIR" ]; then
  if [ -w /usr/local/bin ] 2>/dev/null; then
    BIN_DIR=/usr/local/bin
  else
    BIN_DIR="$HOME/.local/bin"
  fi
fi
mkdir -p "$BIN_DIR" || die "cannot create $BIN_DIR"

archive="${BIN}_${strip_v}_${os}_${arch}.tar.gz"
base="https://github.com/$REPO/releases/download/$VERSION"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT INT TERM

echo "install: fetching $BIN $VERSION ($os/$arch)"
fetch_to "$base/$archive" "$tmp/$archive" || die "download failed: $base/$archive"

# Verify against the release checksums. This is the only integrity check
# between GitHub and your PATH, so a missing checksum file is fatal.
fetch_to "$base/checksums.txt" "$tmp/checksums.txt" || die "could not fetch checksums.txt"
if command -v sha256sum >/dev/null 2>&1; then
  want=$(grep " $archive\$" "$tmp/checksums.txt" | awk '{print $1}')
  got=$(sha256sum "$tmp/$archive" | awk '{print $1}')
elif command -v shasum >/dev/null 2>&1; then
  want=$(grep " $archive\$" "$tmp/checksums.txt" | awk '{print $1}')
  got=$(shasum -a 256 "$tmp/$archive" | awk '{print $1}')
else
  die "neither sha256sum nor shasum is available; refusing to install unverified"
fi
[ -n "$want" ] || die "no checksum listed for $archive"
[ "$want" = "$got" ] || die "checksum mismatch for $archive"

tar -xzf "$tmp/$archive" -C "$tmp"
install -m 0755 "$tmp/$BIN" "$BIN_DIR/$BIN" 2>/dev/null \
  || { cp "$tmp/$BIN" "$BIN_DIR/$BIN" && chmod 0755 "$BIN_DIR/$BIN"; }

echo "install: $BIN $VERSION installed to $BIN_DIR/$BIN"
case ":$PATH:" in
  *":$BIN_DIR:"*) ;;
  *) echo "install: note — $BIN_DIR is not on your PATH" ;;
esac
