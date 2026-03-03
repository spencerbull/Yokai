#!/bin/sh
set -e

# yokai installer
# Usage: curl -fsSL https://raw.githubusercontent.com/spencerbull/Yokai/main/install.sh | sh

REPO="spencerbull/Yokai"
INSTALL_DIR="/usr/local/bin"
FALLBACK_INSTALL_DIR="$HOME/.local/bin"
BINARY="yokai"
PROJECT_NAME="Yokai"

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
FILENAME="${PROJECT_NAME}_${VERSION}_${OS}_${ARCH}.tar.gz"
URL="https://github.com/$REPO/releases/download/v${VERSION}/${FILENAME}"

TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT

echo "Downloading from: $URL"
curl -fsSL "$URL" -o "${TMP_DIR}/${FILENAME}"
tar -xzf "${TMP_DIR}/${FILENAME}" -C "$TMP_DIR"

# Install binary
USE_SUDO=0
if [ -w "$INSTALL_DIR" ]; then
  TARGET_DIR="$INSTALL_DIR"
elif command -v sudo >/dev/null 2>&1 && sudo -n true >/dev/null 2>&1; then
  TARGET_DIR="$INSTALL_DIR"
  USE_SUDO=1
else
  TARGET_DIR="$FALLBACK_INSTALL_DIR"
fi

if [ "$TARGET_DIR" = "$FALLBACK_INSTALL_DIR" ]; then
  mkdir -p "$TARGET_DIR"
  mv "${TMP_DIR}/${BINARY}" "${TARGET_DIR}/${BINARY}"
else
  if [ "$USE_SUDO" -eq 1 ]; then
    sudo mv "${TMP_DIR}/${BINARY}" "${TARGET_DIR}/${BINARY}"
    sudo chmod +x "${TARGET_DIR}/${BINARY}"
  else
    mv "${TMP_DIR}/${BINARY}" "${TARGET_DIR}/${BINARY}"
    chmod +x "${TARGET_DIR}/${BINARY}"
  fi
fi

if [ "$TARGET_DIR" = "$FALLBACK_INSTALL_DIR" ]; then
  chmod +x "${TARGET_DIR}/${BINARY}"

  for rc_file in "$HOME/.bashrc" "$HOME/.zshrc"; do
    if [ ! -f "$rc_file" ]; then
      continue
    fi
    if ! grep -Fq 'export PATH="$HOME/.local/bin:$PATH"' "$rc_file"; then
      printf '\n# Added by yokai installer\nexport PATH="$HOME/.local/bin:$PATH"\n' >> "$rc_file"
      echo "Updated PATH in $rc_file"
    fi
  done
fi

echo "yokai v${VERSION} installed to ${TARGET_DIR}/${BINARY}"
echo "Run 'yokai --help' to get started!"

# Verify installation
if command -v yokai >/dev/null 2>&1; then
  echo "Installation verified: $(yokai --version 2>/dev/null || echo 'yokai is ready')"
else
  echo "Warning: ${TARGET_DIR} may not be in your PATH"
  echo "Add ${TARGET_DIR} to your PATH or run: export PATH=\$PATH:${TARGET_DIR}"
fi
