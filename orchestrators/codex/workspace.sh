#!/usr/bin/env bash
# workspace.sh — workspace phase for codex.
# Clones (or updates) the configured git repository into TAXIWAY_WORKSPACE_DIR.
# No-op when TAXIWAY_REPO_URL is not set.

set -euo pipefail

# shellcheck source=../../infra/trace/events.sh
source "$(dirname "${BASH_SOURCE[0]}")/../../infra/trace/events.sh" 2>/dev/null || true
# shellcheck source=../../infra/workspace/clone.sh
source "$(dirname "${BASH_SOURCE[0]}")/../../infra/workspace/clone.sh"
lab_emit_event phase start

if [[ -z "${TAXIWAY_REPO_URL:-}" ]]; then
    echo "No repo configured for this lab — skipping workspace phase"
    lab_emit_event phase done
    exit 0
fi

# If Taxiway prepared an isolated workspace repo, clone from it.
if [[ -n "${TAXIWAY_REPO_FORK_URL:-}" ]]; then
    TAXIWAY_REPO_URL="$TAXIWAY_REPO_FORK_URL"
    export TAXIWAY_REPO_URL
fi

workspace_clone

lab_emit_event phase done
