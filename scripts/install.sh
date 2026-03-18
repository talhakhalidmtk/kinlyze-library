#!/usr/bin/env sh
# Kinlyze installer — curl -sSL https://kinlyze.com/install.sh | sh
# Supports: macOS (Intel + Apple Silicon), Linux (amd64 + arm64)
set -e

REPO="talhakhalidmtk/kinlyze-library"
BINARY="kinlyze"
INSTALL_DIR="${KINLYZE_INSTALL_DIR:-/usr/local/bin}"
GITHUB_API="https://api.github.com/repos/${REPO}/releases/latest"

# ── Detect OS and arch ────────────────────────────────────────────────────────

detect_os() {
  case "$(uname -s)" in
    Darwin) echo "darwin" ;;
    Linux)  echo "linux"  ;;
    *)      echo "unsupported OS: $(uname -s)"; exit 1 ;;
  esac
}

detect_arch() {
  case "$(uname -m)" in
    x86_64)          echo "amd64" ;;
    arm64|aarch64)   echo "arm64" ;;
    *)               echo "unsupported arch: $(uname -m)"; exit 1 ;;
  esac
}

OS=$(detect_os)
ARCH=$(detect_arch)

# ── Get latest version ────────────────────────────────────────────────────────

echo "→ Fetching latest release..."
if command -v curl > /dev/null 2>&1; then
  LATEST=$(curl -sSfL "${GITHUB_API}" | grep '"tag_name"' | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')
elif command -v wget > /dev/null 2>&1; then
  LATEST=$(wget -qO- "${GITHUB_API}" | grep '"tag_name"' | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')
else
  echo "Error: curl or wget is required."
  exit 1
fi

if [ -z "$LATEST" ]; then
  echo "Error: Could not determine latest version."
  exit 1
fi

VERSION="${LATEST#v}"
echo "→ Latest version: ${LATEST}"

# ── Download ──────────────────────────────────────────────────────────────────

ARCHIVE_NAME="${BINARY}_${VERSION}_${OS}_${ARCH}.tar.gz"
DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${LATEST}/${ARCHIVE_NAME}"
CHECKSUM_URL="https://github.com/${REPO}/releases/download/${LATEST}/checksums.txt"

TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT

echo "→ Downloading ${ARCHIVE_NAME}..."
if command -v curl > /dev/null 2>&1; then
  curl -sSfL "${DOWNLOAD_URL}"  -o "${TMP_DIR}/${ARCHIVE_NAME}"
  curl -sSfL "${CHECKSUM_URL}"  -o "${TMP_DIR}/checksums.txt"
else
  wget -qO "${TMP_DIR}/${ARCHIVE_NAME}" "${DOWNLOAD_URL}"
  wget -qO "${TMP_DIR}/checksums.txt"  "${CHECKSUM_URL}"
fi

# ── Verify checksum ───────────────────────────────────────────────────────────

echo "→ Verifying checksum..."
cd "${TMP_DIR}"
if command -v sha256sum > /dev/null 2>&1; then
  grep "${ARCHIVE_NAME}" checksums.txt | sha256sum --check --status
elif command -v shasum > /dev/null 2>&1; then
  grep "${ARCHIVE_NAME}" checksums.txt | shasum -a 256 --check --status
else
  echo "Warning: Could not verify checksum (sha256sum/shasum not found). Proceeding anyway."
fi

# ── Extract and install ───────────────────────────────────────────────────────

echo "→ Extracting..."
tar -xzf "${ARCHIVE_NAME}" -C "${TMP_DIR}"

echo "→ Installing to ${INSTALL_DIR}/${BINARY}..."
if [ -w "${INSTALL_DIR}" ]; then
  cp "${TMP_DIR}/${BINARY}" "${INSTALL_DIR}/${BINARY}"
  chmod +x "${INSTALL_DIR}/${BINARY}"
else
  sudo cp "${TMP_DIR}/${BINARY}" "${INSTALL_DIR}/${BINARY}"
  sudo chmod +x "${INSTALL_DIR}/${BINARY}"
fi

# ── Verify ────────────────────────────────────────────────────────────────────

echo ""
echo "✓ Installed kinlyze ${LATEST}"
echo ""
kinlyze version
echo ""
echo "  Run: kinlyze --help"
echo "  Docs: https://kinlyze.com"
