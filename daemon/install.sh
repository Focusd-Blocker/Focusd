#!/usr/bin/env bash
set -euo pipefail

# focusd install.sh — first-time build + systemd user unit setup
#
# Run from the project root (the directory containing go.mod / main.go).

BIN_NAME="focusd"
INSTALL_DIR="${HOME}/.local/bin"
UNIT_DIR="${HOME}/.config/systemd/user"
UNIT_NAME="focusd.service"
CACHE_DIR="${HOME}/.cache/focusd"

echo "==> Building ${BIN_NAME}"
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o "${BIN_NAME}" .

echo "==> Installing binary to ${INSTALL_DIR}"
mkdir -p "${INSTALL_DIR}"
cp "${BIN_NAME}" "${INSTALL_DIR}/${BIN_NAME}"
chmod +x "${INSTALL_DIR}/${BIN_NAME}"

echo "==> Ensuring cache dir exists at ${CACHE_DIR}"
mkdir -p "${CACHE_DIR}"

echo "==> Downloading focusd extension (.xpi)"
EXTENSION_DIR="${CACHE_DIR}/extension"
mkdir -p "${EXTENSION_DIR}"
curl -fSL --retry 3 \
    -o "${EXTENSION_DIR}/focusd.xpi" \
    "https://github.com/Focusd-Blocker/Focusd/releases/latest/download/focusd.xpi"

XPI_PATH="${EXTENSION_DIR}/focusd.xpi"
XPI_URL="file://${XPI_PATH}"
POLICY_ID="{5f1c1d4d-c0f0-41bc-862b-0c7f8b860beb}"

POLICY_JSON=$(cat <<POLICYEOF
{
  "policies": {
    "ExtensionSettings": {
      "${POLICY_ID}": {
        "installation_mode": "force_installed",
        "install_url": "${XPI_URL}",
        "private_browsing": true
      }
    }
  }
}
POLICYEOF
)

echo "==> Deploying Firefox policies to detected browsers (requires sudo)"
POLICY_FILE="policies.json"

write_policy() {
    local dir="$1"
    mkdir -p "${dir}" 2>/dev/null || sudo mkdir -p "${dir}"
    echo "${POLICY_JSON}" > "/tmp/focusd_policy_$$.json"
    if cp "/tmp/focusd_policy_$$.json" "${dir}/${POLICY_FILE}" 2>/dev/null; then
        echo "  wrote ${dir}/${POLICY_FILE}"
    elif sudo cp "/tmp/focusd_policy_$$.json" "${dir}/${POLICY_FILE}" 2>/dev/null; then
        echo "  wrote ${dir}/${POLICY_FILE} (via sudo)"
    else
        echo "  skipped ${dir}/${POLICY_FILE} (permission denied)"
    fi
    rm -f "/tmp/focusd_policy_$$.json"
}

# Firefox
for dir in \
    "/usr/lib/firefox/distribution" \
    "/usr/lib64/firefox/distribution" \
    "/usr/lib/firefox-esr/distribution" \
    "/opt/firefox/distribution" \
    "/etc/firefox/policies"; do
    parent="$(dirname "${dir}")"
    [ -d "${parent}" ] && write_policy "${dir}"
done

# LibreWolf
for dir in \
    "/usr/lib/librewolf/distribution" \
    "/usr/lib64/librewolf/distribution" \
    "/etc/librewolf/policies"; do
    parent="$(dirname "${dir}")"
    [ -d "${parent}" ] && write_policy "${dir}"
done

# Waterfox
for dir in \
    "/usr/lib/waterfox/distribution" \
    "/usr/lib64/waterfox/distribution"; do
    parent="$(dirname "${dir}")"
    [ -d "${parent}" ] && write_policy "${dir}"
done

# Floorp
for dir in \
    "/usr/lib/floorp/distribution" \
    "/usr/lib64/floorp/distribution"; do
    parent="$(dirname "${dir}")"
    [ -d "${parent}" ] && write_policy "${dir}"
done

# Snap Firefox
if [ -d "/snap/firefox/current" ]; then
    write_policy "/snap/firefox/current/distribution"
fi

# Flatpak Firefox
if [ -d "${HOME}/.var/app/org.mozilla.firefox" ]; then
    write_policy "${HOME}/.var/app/org.mozilla.firefox/.mozilla/firefox/distribution"
fi

# Flatpak LibreWolf
if [ -d "${HOME}/.var/app/io.gitlab.librewolf-community" ]; then
    write_policy "${HOME}/.var/app/io.gitlab.librewolf-community/.librewolf/distribution"
fi

echo "==> Writing systemd user unit to ${UNIT_DIR}/${UNIT_NAME}"
mkdir -p "${UNIT_DIR}"
cat > "${UNIT_DIR}/${UNIT_NAME}" <<'EOF'
[Unit]
Description=focusd - focus daemon
After=network-online.target
Wants=network-online.target
RefuseManualStop=yes


[Service]
Type=simple
ExecStart=%h/.local/bin/focusd
Restart=always
RestartSec=2

# Hardening - loopback-only local service, no reason it needs much
NoNewPrivileges=yes
PrivateTmp=yes
ProtectSystem=strict
ProtectHome=read-only
ReadWritePaths=%h/.cache/focusd
ProtectKernelTunables=yes
ProtectKernelModules=yes
ProtectControlGroups=yes
RestrictAddressFamilies=AF_INET AF_INET6 AF_UNIX
RestrictNamespaces=yes
LockPersonality=yes
MemoryDenyWriteExecute=yes

[Install]
WantedBy=default.target
EOF

echo "==> Reloading systemd user units"
systemctl --user daemon-reload

echo "==> Enabling and starting ${UNIT_NAME}"
systemctl --user enable --now "${UNIT_NAME}"

echo "==> Done. Checking status:"
sleep 1
systemctl --user status "${UNIT_NAME}" --no-pager || true

echo
echo "Try: curl http://127.0.0.1:36287/blocklist"
