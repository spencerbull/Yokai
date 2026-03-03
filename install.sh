#!/bin/sh
set -e

# yokai installer
# Usage: curl -fsSL https://raw.githubusercontent.com/spencerbull/yokai/main/install.sh | sh

REPO="spencerbull/yokai"
INSTALL_DIR="/usr/local/bin"
BINARY="yokai"

# Detect OS and architecture
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

case "$ARCH" in
  x86_64|amd64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

case "$OS" in
  linux|darwin) ;;
  *) echo "Unsupported OS: $OS"; exit 1 ;;
esac

# Get latest version from GitHub API
VERSION=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" | grep '"tag_name"' | sed -E 's/.*"v([^"]+)".*/\1/')

if [ -z "$VERSION" ]; then
  echo "Error: Could not determine latest version"
  exit 1
fi

echo "Installing yokai v${VERSION} for ${OS}/${ARCH}..."

# Download and extract
FILENAME="${BINARY}_${VERSION}_${OS}_${ARCH}.tar.gz"
URL="https://github.com/$REPO/releases/download/v${VERSION}/${FILENAME}"

TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT

echo "Downloading from: $URL"
curl -fsSL "$URL" -o "${TMP_DIR}/${FILENAME}"
tar -xzf "${TMP_DIR}/${FILENAME}" -C "$TMP_DIR"

# Install binary
if [ -w "$INSTALL_DIR" ]; then
  mv "${TMP_DIR}/${BINARY}" "${INSTALL_DIR}/${BINARY}"
else
  echo "Requires sudo to install to ${INSTALL_DIR}"
  sudo mv "${TMP_DIR}/${BINARY}" "${INSTALL_DIR}/${BINARY}"
fi

chmod +x "${INSTALL_DIR}/${BINARY}"

echo "yokai v${VERSION} installed to ${INSTALL_DIR}/${BINARY}"
echo "Run 'yokai --help' to get started!"

# Verify installation
if command -v yokai >/dev/null 2>&1; then
  echo "Installation verified: $(yokai --version 2>/dev/null || echo 'yokai is ready')"
else
  echo "Warning: ${INSTALL_DIR} may not be in your PATH"
  echo "Add ${INSTALL_DIR} to your PATH or run: export PATH=\$PATH:${INSTALL_DIR}"
fi
