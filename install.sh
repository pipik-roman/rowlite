#!/bin/sh
set -e

REPO="pipik-roman/rowlite"
INSTALL_DIR="/usr/local/bin"

# Detect OS and Architecture
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"

case "$ARCH" in
  x86_64|amd64) ARCH="amd64" ;;
  arm64|aarch64) ARCH="arm64" ;;
  *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

# Fetch latest release tag
TAG=$(curl -s https://api.github.com/repos/$REPO/releases/latest | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')

if [ -z "$TAG" ]; then
  echo "Failed to fetch latest release version."
  exit 1
fi

URL="https://github.com/$REPO/releases/download/${TAG}/rowlite_${TAG#v}_${OS}_${ARCH}.tar.gz"

echo "Downloading Rowlite ${TAG} for ${OS}/${ARCH}..."
TMP_DIR=$(mktemp -d)
curl -sL "$URL" | tar -xz -C "$TMP_DIR"

echo "Installing to ${INSTALL_DIR} (may require sudo)..."
if [ -w "$INSTALL_DIR" ]; then
  mv "$TMP_DIR/rowlite" "$INSTALL_DIR/"
else
  sudo mv "$TMP_DIR/rowlite" "$INSTALL_DIR/"
fi

rm -rf "$TMP_DIR"
echo "Rowlite installed successfully!"
rowlite --version
