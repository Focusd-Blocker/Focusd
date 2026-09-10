#!/usr/bin/env bash
set -euo pipefail

# focusd update.sh — rebuild + restart during active development/testing.
#
# Note: the unit has RefuseManualStop=yes, so a plain
# `systemctl --user restart` will be refused. We use `kill` to stop it
# (that bypasses RefuseManualStop) then `start` fresh.
#
# Run from the project root (the directory containing go.mod / main.go).

BIN_NAME="focusd"
INSTALL_DIR="${HOME}/.local/bin"
UNIT_NAME="focusd.service"
CACHE_DIR="${HOME}/.cache/focusd"
EXTENSION_DIR="${CACHE_DIR}/extension"

echo "==> Building ${BIN_NAME}"
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o "${BIN_NAME}" .

echo "==> Installing updated binary to ${INSTALL_DIR}"
# mv does an atomic rename, which works even while the old binary is still
# running (unlike cp, which truncates the destination in place and gets
# "Text file busy" from a Restart=always service that's always running).
chmod +x "${BIN_NAME}"
mv "${BIN_NAME}" "${INSTALL_DIR}/${BIN_NAME}"

echo "==> Checking for extension updates"
mkdir -p "${EXTENSION_DIR}"
OLD_HASH=""
if [ -f "${EXTENSION_DIR}/focusd.xpi" ]; then
    OLD_HASH=$(sha256sum "${EXTENSION_DIR}/focusd.xpi" | cut -d' ' -f1)
fi
EXTENSION_TAG="$(
    curl -fsSL --retry 3 \
        "https://api.github.com/repos/Focusd-Blocker/Focusd/releases?per_page=100" |
        python3 -c '
import json
import sys

releases = json.load(sys.stdin)
for release in releases:
    tag = release.get("tag_name", "")
    assets = {asset.get("name") for asset in release.get("assets", [])}
    if tag.startswith("extension-v") or (tag.startswith("v") and "focusd.xpi" in assets):
        print(tag)
        break
else:
    raise SystemExit("no extension release found")
'
)"
curl -fSL --retry 3 \
    -o "${EXTENSION_DIR}/focusd.xpi" \
    "https://github.com/Focusd-Blocker/Focusd/releases/download/${EXTENSION_TAG}/focusd.xpi"
NEW_HASH=$(sha256sum "${EXTENSION_DIR}/focusd.xpi" | cut -d' ' -f1)

if [ "$OLD_HASH" != "$NEW_HASH" ]; then
    echo "==> Extension updated, redeploying Firefox policies"
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

    write_policy() {
        local dir="$1"
        mkdir -p "${dir}" 2>/dev/null || sudo mkdir -p "${dir}"
        echo "${POLICY_JSON}" > "/tmp/focusd_policy_$$.json"
        if cp "/tmp/focusd_policy_$$.json" "${dir}/policies.json" 2>/dev/null; then
            echo "  wrote ${dir}/policies.json"
        elif sudo cp "/tmp/focusd_policy_$$.json" "${dir}/policies.json" 2>/dev/null; then
            echo "  wrote ${dir}/policies.json (via sudo)"
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
else
    echo "  extension unchanged, skipping policy redeploy"
fi

echo "==> Stopping running instance (via kill, bypasses RefuseManualStop)"
systemctl --user kill "${UNIT_NAME}" 2>/dev/null || true

# Give it a moment to actually exit before starting again
sleep 0.5

echo "==> Starting ${UNIT_NAME}"
systemctl --user start "${UNIT_NAME}"

echo "==> Status:"
sleep 0.5
systemctl --user status "${UNIT_NAME}" --no-pager || true

echo
echo "==> Tailing logs (Ctrl+C to exit)"
journalctl --user -u "${UNIT_NAME}" -f
