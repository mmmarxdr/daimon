#!/bin/sh
# Daimon installer — downloads the latest release binary.
# Usage: curl -fsSL https://raw.githubusercontent.com/mmmarxdr/micro-claw/main/install.sh | sh
# Note: the GitHub repo URL will be updated to mmmarxdr/daimon after the repo rename (Phase 5).
set -e

REPO="mmmarxdr/micro-claw"
INSTALL_DIR="/usr/local/bin"
BINARY="daimon"

# Detect OS.
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$OS" in
  linux)  OS="linux" ;;
  darwin) OS="darwin" ;;
  *)      echo "Unsupported OS: $OS"; exit 1 ;;
esac

# Detect architecture.
ARCH=$(uname -m)
case "$ARCH" in
  x86_64|amd64)  ARCH="amd64" ;;
  aarch64|arm64)  ARCH="arm64" ;;
  *)              echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

# Get latest version from GitHub API.
VERSION=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | sed -E 's/.*"v([^"]+)".*/\1/')
if [ -z "$VERSION" ]; then
  echo "Failed to fetch latest version."
  exit 1
fi

FILENAME="${BINARY}_${VERSION}_${OS}_${ARCH}.tar.gz"
URL="https://github.com/${REPO}/releases/download/v${VERSION}/${FILENAME}"

echo "Installing daimon v${VERSION} (${OS}/${ARCH})..."

# Download and extract to temp dir.
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

curl -fsSL "$URL" -o "${TMP}/${FILENAME}"
tar -xzf "${TMP}/${FILENAME}" -C "$TMP"

# Install binary.
if [ -w "$INSTALL_DIR" ]; then
  mv "${TMP}/${BINARY}" "${INSTALL_DIR}/${BINARY}"
else
  echo "Need sudo to install to ${INSTALL_DIR}"
  sudo mv "${TMP}/${BINARY}" "${INSTALL_DIR}/${BINARY}"
fi
chmod +x "${INSTALL_DIR}/${BINARY}"

echo ""
echo "  Installed: ${INSTALL_DIR}/${BINARY} (v${VERSION})"
echo ""
echo "  Get started:"
echo "    daimon web        # Start dashboard + setup wizard"
echo "    daimon --setup    # Terminal setup wizard"
echo ""
