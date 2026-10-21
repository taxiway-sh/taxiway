#!/usr/bin/env bash
# stop.sh — stop hook for gastown.
#
# Stops the Gas Town services of the HQ so the workspace can be cleared.

set -euo pipefail

export PATH="$HOME/.local/bin:$PATH"

# shellcheck source=../../infra/commands/steps.sh
source "$(dirname "${BASH_SOURCE[0]}")/../../infra/commands/steps.sh"

log() { printf '\n\033[1;34m[gastown-stop]\033[0m %s\n' "$*"; }

HQ_DIR="${TAXIWAY_HQ_DIR:-/lab/work/gt}"

log "Stopping Gas Town services when present"
if taxiway_is_plan || [[ ! -d "$HQ_DIR" ]] || ! command -v gt >/dev/null 2>&1; then
    exit 0
fi
(cd "$HQ_DIR" && gt down --all) || true
