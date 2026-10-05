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

# Local upload-pack is a separate Git process and needs guest-global trust
# for the exact read-only source owned by the host. Never trust a wildcard.
for trusted in "$mirror" "/lab/git-source/$name"; do
    found=false
    while IFS= read -r configured; do
        if [[ "$configured" == "$trusted" ]]; then
            found=true
            break
        fi
    done < <(git config --global --get-all safe.directory || true)
    if [[ "$found" == false ]]; then
        git config --global --add safe.directory "$trusted"
    fi
    echo "Trusted Taxiway-managed Git mirror: $trusted"
done
