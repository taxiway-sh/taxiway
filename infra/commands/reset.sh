#!/usr/bin/env bash
# Wipe ephemeral lab state so a run can start from a known-clean slate.
# Only touches /lab/work — never the ro mounts.
#
# Under the cloisoned layout, all in-lab scratch lives under /lab/work.
# Contents are wiped; .taxiway-* markers are preserved so phase tracking
# survives a reset.

set -euo pipefail

target="${LAB_RESET_TARGET:-/lab/work}"

echo "This will delete the contents of: $target"

if [ "${LAB_RESET_YES:-}" != "1" ]; then
  read -r -p "Proceed? [y/N] " reply
  case "$reply" in
    y|Y|yes|YES) ;;
    *) echo "Aborted."; exit 1;;
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
