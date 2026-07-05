#!/usr/bin/env bash
# workspace-clone.sh — shared helper for workspace phase scripts.
#
# Provides: workspace_clone()
#
# Clones TAXIWAY_REPO_URL into TAXIWAY_WORKSPACE_DIR (idempotent):
#   - If .git already present → fetch all remotes and optionally checkout TAXIWAY_REPO_REF.
#   - If not present → git clone [--branch TAXIWAY_REPO_REF] TAXIWAY_REPO_URL TAXIWAY_WORKSPACE_DIR.
#
# Security: git is invoked with -c protocol.file.allow=never except for
# Taxiway-managed file:///lab/git/*.git remotes prepared on the host.
#
# Environment variables consumed:
#   TAXIWAY_REPO_URL        (required) — git URL of the repository to clone.
#   TAXIWAY_REPO_REF        (optional) — branch, tag, or SHA to check out.
#   TAXIWAY_WORKSPACE_DIR   (required) — destination path inside the lab.

_git_protocol_opts_for_url() {
    local url="$1"
    if [[ "$url" =~ ^file:///lab/git/[^/]+\.git$ ]]; then
        GIT_PROTOCOL_OPTS=(-c protocol.file.allow=always)
    else
        GIT_PROTOCOL_OPTS=(-c protocol.file.allow=never)
    fi
}

workspace_clone() {
    : "${TAXIWAY_REPO_URL:?workspace_clone: TAXIWAY_REPO_URL is required}"
    : "${TAXIWAY_WORKSPACE_DIR:?workspace_clone: TAXIWAY_WORKSPACE_DIR is required}"

    local git_opts
    _git_protocol_opts_for_url "$TAXIWAY_REPO_URL"
    git_opts=("${GIT_PROTOCOL_OPTS[@]}")

    mkdir -p "$(dirname "$TAXIWAY_WORKSPACE_DIR")"

    if [[ -d "$TAXIWAY_WORKSPACE_DIR/.git" ]]; then
        echo "Workspace already cloned at $TAXIWAY_WORKSPACE_DIR — fetching updates"
        git "${git_opts[@]}" -C "$TAXIWAY_WORKSPACE_DIR" fetch --all --prune
        if [[ -n "${TAXIWAY_REPO_REF:-}" ]]; then
            git "${git_opts[@]}" -C "$TAXIWAY_WORKSPACE_DIR" checkout "$TAXIWAY_REPO_REF"
            git "${git_opts[@]}" -C "$TAXIWAY_WORKSPACE_DIR" pull --ff-only \
              || echo "WARN: could not fast-forward $TAXIWAY_WORKSPACE_DIR — working tree may be behind" >&2
        fi
        return 0
    fi

    echo "Cloning $TAXIWAY_REPO_URL into $TAXIWAY_WORKSPACE_DIR"
    if [[ -n "${TAXIWAY_REPO_REF:-}" ]]; then
        git "${git_opts[@]}" clone --branch "$TAXIWAY_REPO_REF" "$TAXIWAY_REPO_URL" "$TAXIWAY_WORKSPACE_DIR"
    else
        git "${git_opts[@]}" clone "$TAXIWAY_REPO_URL" "$TAXIWAY_WORKSPACE_DIR"
    fi
}
