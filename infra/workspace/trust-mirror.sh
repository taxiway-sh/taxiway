#!/usr/bin/env bash
# trust-mirror.sh — trust the exact Taxiway-managed bare Git mirror for this Lab.

set -euo pipefail

url="${TAXIWAY_REPO_FORK_URL:-}"
case "$url" in
    file:///lab/git/*.git)
        mirror="${url#file://}"
        name="${mirror#/lab/git/}"
        if [[ -z "$name" || "$name" == */* ]]; then
            echo "ERROR: invalid Taxiway-managed Git mirror: $url" >&2
            exit 1
        fi
        ;;
    *)
        echo "ERROR: invalid Taxiway-managed Git mirror: ${url:-<unset>}" >&2
        exit 1
        ;;
esac

while IFS= read -r configured; do
    if [[ "$configured" == "$mirror" ]]; then
        echo "Git mirror already trusted: $mirror"
        exit 0
    fi
done < <(git config --global --get-all safe.directory || true)

git config --global --add safe.directory "$mirror"
echo "Trusted Taxiway-managed Git mirror: $mirror"
