#!/usr/bin/env bash
# workspace.sh — workspace phase for claude-code.
# Clones (or updates) the configured git repository into TAXIWAY_WORKSPACE_DIR.
# No-op when TAXIWAY_REPO_URL is not set.

set -euo pipefail

# shellcheck source=../../infra/trace/events.sh
source "$(dirname "${BASH_SOURCE[0]}")/../../infra/trace/events.sh" 2>/dev/null || true
# shellcheck source=../../infra/commands/steps.sh
source "$(dirname "${BASH_SOURCE[0]}")/../../infra/commands/steps.sh"

log() { printf '\n\033[1;34m[claude-code-workspace]\033[0m %s\n' "$*"; }

if [[ -z "${TAXIWAY_REPO_URL:-}" ]]; then
    echo "No repo configured for this lab — skipping workspace phase"
    exit 0
fi

# If Taxiway prepared an isolated workspace repo, clone from it.
if [[ -n "${TAXIWAY_REPO_FORK_URL:-}" ]]; then
    TAXIWAY_REPO_URL="$TAXIWAY_REPO_FORK_URL"
    export TAXIWAY_REPO_URL
fi

if taxiway_is_plan; then
    log "Preparing local repository mirror"
    taxiway_plan_detail "$TAXIWAY_REPO_URL"
    if taxiway_can_inspect && [[ -d "${TAXIWAY_WORKSPACE_DIR:-}/.git" ]]; then
        log "Updating workspace repository"
    else
        log "Cloning workspace repository"
    fi
    taxiway_plan_detail "${TAXIWAY_WORKSPACE_DIR:-/lab/work/<repository>}"
    if [[ -n "${TAXIWAY_REPO_REF:-}" ]]; then
        log "Checking out workspace ref ${TAXIWAY_REPO_REF}"
    fi
    exit 0
fi

# shellcheck source=../../infra/workspace/clone.sh
source "$(dirname "${BASH_SOURCE[0]}")/../../infra/workspace/clone.sh"
lab_emit_event phase start

workspace_clone

lab_emit_event phase done
