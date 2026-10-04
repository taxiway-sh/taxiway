#!/usr/bin/env bash
# Wipe ephemeral lab state so a run can start from a known-clean slate.
# Only touches /lab/work — never the ro mounts.
#
# Under the cloisoned layout, all in-lab scratch lives under /lab/work.
# Contents are wiped; .taxiway-* markers are preserved so phase tracking
# survives a reset.

set -euo pipefail

# shellcheck source=steps.sh
source "$(dirname "${BASH_SOURCE[0]}")/steps.sh"

target="${LAB_RESET_TARGET:-/lab/work}"

log() { printf '\n\033[1;34m[reset]\033[0m %s\n' "$*"; }

if taxiway_is_plan; then
  log "Stopping workspace services"
  taxiway_plan_detail "Gas Town services when present"
  log "Clearing $target contents"
  taxiway_plan_detail "preserving .gitkeep and .taxiway-* markers"
  exit 0
fi

echo "This will delete the contents of: $target"

if [ "${LAB_RESET_YES:-}" != "1" ]; then
  read -r -p "Proceed? [y/N] " reply || reply=""
  case "$reply" in
    y|Y|yes|YES) ;;
    *) echo "Aborted."; exit 0;;
  esac
fi

mkdir -p "$target"

gastown_hq="$target/gt"
if [ -d "$gastown_hq" ] && command -v gt >/dev/null 2>&1; then
  (cd "$gastown_hq" && gt down --all) || true
fi

# Remove everything inside, preserving .taxiway-* markers and .gitkeep stubs.
find "$target" -mindepth 1 -maxdepth 1 ! -name '.gitkeep' ! -name '.taxiway-*' -exec rm -rf {} +

echo "Reset complete."
