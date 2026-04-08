#!/usr/bin/env bash
set -euo pipefail

REPO="twistedogic/tcr"
BINARY="tcr"
INSTALL_DIR="/usr/local/bin"
SERVICE_NAME="tcr"
SERVICE_FILE="/etc/systemd/system/${SERVICE_NAME}.service"

# --- helpers ---
info()  { echo "[tcr] $*"; }
error() { echo "[tcr] ERROR: $*" >&2; exit 1; }
need()  { command -v "$1" &>/dev/null || error "required command not found: $1"; }

need curl
need tar
need systemctl

# --- detect OS/arch ---
OS="$(uname -s)"
ARCH="$(uname -m)"

case "$OS" in
  Linux)  OS_NAME="Linux" ;;
  Darwin) OS_NAME="Darwin" ;;
  *)      error "unsupported OS: $OS" ;;
esac

case "$ARCH" in
  x86_64)          ARCH_NAME="x86_64" ;;
  aarch64|arm64)   ARCH_NAME="arm64" ;;
  *)               error "unsupported architecture: $ARCH" ;;
esac

# --- fetch latest release ---
info "fetching latest release from github.com/${REPO} ..."
LATEST_TAG="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
  | grep '"tag_name"' | head -1 | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')"

[ -n "$LATEST_TAG" ] || error "could not determine latest release tag"
info "latest version: ${LATEST_TAG}"

ARCHIVE="${BINARY}_${OS_NAME}_${ARCH_NAME}.tar.gz"
DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${LATEST_TAG}/${ARCHIVE}"

# --- download & install binary ---
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

info "downloading ${DOWNLOAD_URL} ..."
curl -fsSL "$DOWNLOAD_URL" -o "${TMP_DIR}/${ARCHIVE}"
tar -xzf "${TMP_DIR}/${ARCHIVE}" -C "$TMP_DIR"

info "installing binary to ${INSTALL_DIR}/${BINARY} ..."
if [ -w "$INSTALL_DIR" ]; then
  install -m 755 "${TMP_DIR}/${BINARY}" "${INSTALL_DIR}/${BINARY}"
else
  sudo install -m 755 "${TMP_DIR}/${BINARY}" "${INSTALL_DIR}/${BINARY}"
fi

# --- configure service ---
TCR_USER="${TCR_USER:-$(whoami)}"
TCR_HOST="${TCR_HOST:-127.0.0.1}"
TCR_PORT="${TCR_PORT:-2222}"
TCR_INTERVAL="${TCR_INTERVAL:-15m}"
TCR_PASSKEY="${TCR_PASSKEY:-}"

EXEC_START="${INSTALL_DIR}/${BINARY} server -host ${TCR_HOST} -port ${TCR_PORT} -interval ${TCR_INTERVAL}"
[ -n "$TCR_PASSKEY" ] && EXEC_START="${EXEC_START} -passkey ${TCR_PASSKEY}"

info "writing systemd service to ${SERVICE_FILE} ..."
UNIT_CONTENT="[Unit]
Description=TCR SSH server
After=network.target

[Service]
Type=simple
User=${TCR_USER}
ExecStart=${EXEC_START}
Restart=on-failure
RestartSec=5s
Environment=TCR_PASSKEY=${TCR_PASSKEY}

[Install]
WantedBy=multi-user.target
"

if [ -w "$(dirname "$SERVICE_FILE")" ]; then
  printf '%s' "$UNIT_CONTENT" > "$SERVICE_FILE"
else
  printf '%s' "$UNIT_CONTENT" | sudo tee "$SERVICE_FILE" > /dev/null
fi

# --- enable & start ---
info "enabling and starting ${SERVICE_NAME}.service ..."
if [ "$(id -u)" -eq 0 ]; then
  systemctl daemon-reload
  systemctl enable --now "${SERVICE_NAME}.service"
else
  sudo systemctl daemon-reload
  sudo systemctl enable --now "${SERVICE_NAME}.service"
fi

info "done. tcr server is running on ${TCR_HOST}:${TCR_PORT}"
info "  ssh -p ${TCR_PORT} ${TCR_HOST}"
