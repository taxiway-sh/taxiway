#!/usr/bin/env bash
# Keep the writable fork in guest storage, separate from the read-only host source.
set -euo pipefail

workspace_prepare_mirror() {
    local source="$1" mirror="$2"
    mkdir -p "$(dirname "$mirror")"
    if [[ -d "$mirror/objects" ]]; then
        git -c protocol.file.allow=always -c safe.directory="$source" \
            -c safe.directory="$mirror" -C "$mirror" \
            fetch "$source" '+refs/*:refs/*'
    else
        # The transport copies objects: local clones must not create writable
        # hard links or alternates to objects owned by the host.
        git -c protocol.file.allow=always -c safe.directory="$source" \
            clone --no-local --mirror "$source" "$mirror"
    fi
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
    url="${TAXIWAY_REPO_FORK_URL:-}"
    case "$url" in
        file:///lab/git/*.git)
            mirror="${url#file://}"
            name="${mirror#/lab/git/}"
            if [[ -z "$name" || "$name" == */* ]]; then
                echo "ERROR: invalid Taxiway-managed Git mirror" >&2
                exit 1
            fi
            ;;
        *)
            echo "ERROR: invalid Taxiway-managed Git mirror" >&2
            exit 1
            ;;
    esac
    workspace_prepare_mirror "/lab/git-source/$name" "$mirror"
fi
