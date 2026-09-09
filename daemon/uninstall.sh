#!/usr/bin/env bash
set -euo pipefail

BIN_NAME="focusd"
INSTALL_DIR="${HOME}/.local/bin"
UNIT_DIR="${HOME}/.config/systemd/user"
UNIT_NAME="focusd.service"

echo "==> Stopping ${UNIT_NAME}"
systemctl --user stop "${UNIT_NAME}" 2>/dev/null || true

echo "==> Disabling ${UNIT_NAME}"
systemctl --user disable "${UNIT_NAME}" 2>/dev/null || true

echo "==> Removing installed daemon and update files"
rm -f \
    "${INSTALL_DIR}/${BIN_NAME}" \
    "${INSTALL_DIR}/${BIN_NAME}.update" \
    "${INSTALL_DIR}/${BIN_NAME}.updater" \
    "${INSTALL_DIR}/${BIN_NAME}.previous"

echo "==> Removing systemd user unit"
rm -f "${UNIT_DIR}/${UNIT_NAME}"
systemctl --user daemon-reload

echo "==> focusd daemon uninstalled"
echo "    User data and cache were left in place."
