#!/usr/bin/env bash
# workspace.sh — workspace phase for gascity.
# Clones the configured repository, creates the Gas City city when missing and
# registers the repository as a rig. No-op when TAXIWAY_REPO_URL is not set.
#
# Environment variables consumed:
#   TAXIWAY_REPO_URL       (required to activate)
#   TAXIWAY_REPO_FORK_URL  (optional) — lab-local mirror cloned instead
#   TAXIWAY_REPO_REF       (optional) — ref to check out
#   TAXIWAY_REPO_PATH      (optional) — ignored for the rig root; emits a warning
#   TAXIWAY_WORKSPACE_DIR  (required) — clone destination set by Taxiway
#   TAXIWAY_SET_TEMPLATE   — Gas City template for a new city (default gastown)

set -euo pipefail

export PATH="$HOME/.local/bin:$PATH"

# shellcheck source=../../infra/trace/events.sh
source "$(dirname "${BASH_SOURCE[0]}")/../../infra/trace/events.sh" 2>/dev/null || true
# shellcheck source=../../infra/commands/steps.sh
source "$(dirname "${BASH_SOURCE[0]}")/../../infra/commands/steps.sh"
# shellcheck source=lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

log() { printf '\n\033[1;34m[gascity-workspace]\033[0m %s\n' "$*"; }

if [[ -z "${TAXIWAY_REPO_URL:-}" ]]; then
    echo "No repo configured for this lab — skipping workspace phase"
    exit 0
fi

if [[ -n "${TAXIWAY_REPO_FORK_URL:-}" ]]; then
    TAXIWAY_REPO_URL="$TAXIWAY_REPO_FORK_URL"
    export TAXIWAY_REPO_URL
fi

: "${TAXIWAY_WORKSPACE_DIR:?TAXIWAY_WORKSPACE_DIR is required for gascity workspace}"
if [[ -n "${TAXIWAY_REPO_PATH:-}" ]]; then
    echo "WARN: --repo-path is ignored for gascity (the rig is the whole repository)" >&2
fi
RIG_ROOT="$(gascity_rig_root)"
RIG_NAME="$(gascity_rig_name)"

if taxiway_is_plan; then
    if taxiway_can_inspect && [[ -d "$RIG_ROOT/.git" ]]; then
        log "Updating workspace repository"
    else
        log "Cloning workspace repository"
    fi
    taxiway_plan_detail "$RIG_ROOT"
    if [[ -n "${TAXIWAY_REPO_REF:-}" ]]; then
        log "Checking out workspace ref ${TAXIWAY_REPO_REF}"
    fi
    if taxiway_can_inspect && [[ -f "$GASCITY_CITY_DIR/city.toml" ]]; then
        log "Gas City city already initialized at $GASCITY_CITY_DIR"
    else
        log "Initializing Gas City city at $GASCITY_CITY_DIR"
        taxiway_plan_detail "template: $GASCITY_TEMPLATE"
    fi
    log "Adding rig '$RIG_NAME' when missing"
    exit 0
fi

# shellcheck source=../../infra/workspace/clone.sh
source "$(dirname "${BASH_SOURCE[0]}")/../../infra/workspace/clone.sh"
lab_emit_event phase start

TAXIWAY_WORKSPACE_DIR="$RIG_ROOT" workspace_clone

# Gas City commits bead hooks and worktrees in the rig.
if [[ -z "$(git config --global user.email 2>/dev/null)" ]]; then
    git config --global user.email "taxiway@taxiway.local"
    git config --global user.name "taxiway"
fi

gascity_ensure_city
gascity_ensure_rig

lab_emit_event phase done
