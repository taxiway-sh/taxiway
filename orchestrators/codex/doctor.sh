#!/usr/bin/env bash
# Diagnose Codex orchestrator wiring.

set -euo pipefail

source "$(dirname "${BASH_SOURCE[0]}")/../../infra/trace/events.sh" 2>/dev/null || true

lab_emit_event phase start
printf '\n\033[1;34m[codex-orchestrator-doctor]\033[0m tmux session\n'
if tmux has-session -t codex 2>/dev/null; then
    printf '  \033[1;32mOK\033[0m   session codex is running\n'
else
    printf '  \033[1;33mWARN\033[0m session codex is not running\n'
    printf '       Restart it with: taxiway up %s --from start --force\n' "${TAXIWAY_LAB:-<lab>}"
fi
lab_emit_event phase done
