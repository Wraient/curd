#!/data/data/com.termux/files/usr/bin/bash
set -euo pipefail

REPO="${REPO:-wraient/curd}"
ASSET="${ASSET:-curd-android-arm64}"
INSTALL_DIR="${PREFIX:-/data/data/com.termux/files/usr}/bin"
TARGET="${INSTALL_DIR}/curd"
TMP="$(mktemp)"

echo "Downloading ${ASSET} from ${REPO}..."
curl -fsSL "https://github.com/${REPO}/releases/latest/download/${ASSET}" -o "${TMP}"
chmod +x "${TMP}"
mkdir -p "${INSTALL_DIR}"
mv "${TMP}" "${TARGET}"

echo "Installed curd to ${TARGET}"
echo "Run: curd --android-setup"
