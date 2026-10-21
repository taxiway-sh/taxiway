#!/usr/bin/env bash
# Verify the installed Gas City CLI without starting a city or any agent.
# Verifies:
#   1. `gc`, `bd`, `dolt`, `tmux`, `lsof`, `flock` and `jq` are available
#   2. gc, bd and dolt report a version

set -euo pipefail

# shellcheck source=../../infra/commands/steps.sh
source "$(dirname "${BASH_SOURCE[0]}")/../../infra/commands/steps.sh"

log()  { printf '\n\033[1;34m[gascity-verify]\033[0m %s\n' "$*"; }
pass() { printf '  \033[1;32mOK\033[0m   %s\n' "$*"; }
fail() { printf '  \033[1;31mFAIL\033[0m %s\n' "$*" >&2; exit 1; }

export PATH="$HOME/.local/bin:$PATH"

REQUIRED_BINARIES=(gc bd dolt tmux lsof flock jq)

if taxiway_is_plan; then
  log "Verifying required binaries"
  if taxiway_can_inspect; then
    for binary in "${REQUIRED_BINARIES[@]}"; do
      if command -v "$binary" >/dev/null 2>&1; then
        taxiway_plan_detail "$binary is present"
      else
        taxiway_plan_detail "$binary is missing"
      fi
    done
  else
    taxiway_plan_detail "${REQUIRED_BINARIES[*]}"
  fi
  log "Verifying tool versions"
  taxiway_plan_detail "gc bd dolt"
  exit 0
fi

log "binaries"
for binary in "${REQUIRED_BINARIES[@]}"; do
  path="$(command -v "$binary" || true)"
  [[ -n "$path" ]] || fail "$binary not found - run: taxiway install <lab>"
  pass "$binary binary at $path"
done

log "versions"
for binary in gc bd dolt; do
  output="$("$binary" version 2>/dev/null)" || fail "$binary version command failed"
  [[ -n "$output" ]] || fail "$binary version printed nothing"
  pass "$binary: ${output%%$'\n'*}"
done

log "Verify test passed."
