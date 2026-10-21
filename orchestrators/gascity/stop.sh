#!/usr/bin/env bash
# stop.sh — stop hook for gascity.
#
# Stops the city, the Gas City supervisor and its deferred-nudge pollers so
# the workspace can be cleared. The Lima systemd user unit stays installed
# until the lab is removed.

set -euo pipefail

export PATH="$HOME/.local/bin:$PATH"

# shellcheck source=../../infra/commands/steps.sh
source "$(dirname "${BASH_SOURCE[0]}")/../../infra/commands/steps.sh"
# shellcheck source=lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

log() { printf '\n\033[1;34m[gascity-stop]\033[0m %s\n' "$*"; }

log "Stopping the Gas City city, supervisor and nudge pollers when present"
if taxiway_is_plan || ! command -v gc >/dev/null 2>&1; then
    exit 0
fi
gascity_stop_city
