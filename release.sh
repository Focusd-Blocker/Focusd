#!/usr/bin/env bash
set -euo pipefail

usage() {
    echo "Usage: $0 <extension|daemon> <version>" >&2
    exit 1
}

[[ $# -eq 2 ]] || usage

component="$1"
version="$2"
if [[ ! "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+([-.][0-9A-Za-z.-]+)?$ ]]; then
    echo "Invalid version: $version" >&2
    exit 1
fi

case "$component" in
    extension)
        version_file="extension/package.json"
        tag="extension-v${version}"
        node - "$version" <<'NODE'
const fs = require("fs");
const version = process.argv[2];
const path = "extension/package.json";
const packageJSON = JSON.parse(fs.readFileSync(path, "utf8"));
packageJSON.version = version;
fs.writeFileSync(path, JSON.stringify(packageJSON, null, 2) + "\n");
NODE
        ;;
    daemon)
        version_file="daemon/VERSION"
        tag="daemon-v${version}"
        printf '%s\n' "$version" > "$version_file"
        ;;
    *)
        usage
        ;;
esac

unrelated_changes="$(git status --short | grep -vE " ${version_file}$" || true)"
if [[ -n "$unrelated_changes" ]]; then
    echo "Working tree has unrelated changes; commit or stash them first." >&2
    exit 1
fi

if git diff --quiet -- "$version_file" && git diff --cached --quiet -- "$version_file"; then
    echo "${version_file} already has version ${version}" >&2
else
    git add "$version_file"
    git commit -m "Release ${component} v${version}" \
        -m "Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
fi

if git rev-parse --verify --quiet "refs/tags/${tag}" >/dev/null; then
    echo "Tag already exists: ${tag}" >&2
    exit 1
fi

git tag "$tag"
git push origin HEAD "$tag"
echo "Released ${component} v${version} as ${tag}"
