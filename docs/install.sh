#!/bin/sh
set -e

REPO="kunchenguid/treehouse"

# Print the SHA-256 of a file using whichever tool is available
# (sha256sum on Linux, shasum on macOS).
sha256_hash() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | awk '{print $1}'
  else
    echo "Error: need sha256sum or shasum to verify the download" >&2
    return 1
  fi
}

# Prefer ~/.local/bin if it exists and is in PATH (no sudo needed).
# Fall back to /usr/local/bin otherwise.
if echo "$PATH" | tr ':' '\n' | grep -qx "$HOME/.local/bin"; then
  INSTALL_DIR="$HOME/.local/bin"
else
  INSTALL_DIR="/usr/local/bin"
fi

OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"

case "$ARCH" in
  x86_64|amd64) ARCH="amd64" ;;
  arm64|aarch64) ARCH="arm64" ;;
  *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

case "$OS" in
  darwin|linux) ;;
  *) echo "Unsupported OS: $OS"; exit 1 ;;
esac

VERSION="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')"
if [ -z "$VERSION" ]; then
  echo "Failed to determine latest version"
  exit 1
fi

VERSION_NUM="${VERSION#v}"
FILENAME="treehouse-v${VERSION_NUM}-${OS}-${ARCH}.tar.gz"
URL="https://github.com/${REPO}/releases/download/${VERSION}/${FILENAME}"

TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

echo "Downloading treehouse ${VERSION} for ${OS}/${ARCH}..."
curl -fsSL "$URL" -o "${TMPDIR}/${FILENAME}"

# Verify the SHA-256 checksum against the published checksums.txt before
# extracting, matching the integrity check the in-app updater performs.
echo "Verifying checksum..."
curl -fsSL "https://github.com/${REPO}/releases/download/${VERSION}/checksums.txt" -o "${TMPDIR}/checksums.txt"
EXPECTED="$(awk -v f="$FILENAME" '$2 == f {print $1}' "${TMPDIR}/checksums.txt")"
if [ -z "$EXPECTED" ]; then
  echo "No checksum found for ${FILENAME} in checksums.txt"
  exit 1
fi
ACTUAL="$(sha256_hash "${TMPDIR}/${FILENAME}")"
if [ "$EXPECTED" != "$ACTUAL" ]; then
  echo "Checksum verification failed for ${FILENAME}"
  echo "  expected: ${EXPECTED}"
  echo "  actual:   ${ACTUAL}"
  exit 1
fi

tar xzf "${TMPDIR}/${FILENAME}" -C "$TMPDIR"

if [ -w "$INSTALL_DIR" ]; then
  mkdir -p "$INSTALL_DIR"
  mv "${TMPDIR}/treehouse" "${INSTALL_DIR}/treehouse"
else
  echo "Installing to ${INSTALL_DIR} (requires sudo)..."
  sudo mkdir -p "$INSTALL_DIR"
  sudo mv "${TMPDIR}/treehouse" "${INSTALL_DIR}/treehouse"
fi

echo "treehouse ${VERSION} installed to ${INSTALL_DIR}/treehouse"
