#!/usr/bin/env bash
# Install Claude Code orchestrator-level assets.
#
# The `claude` CLI is an agent dependency installed by
# agents/claude-code/install.sh.

set -euo pipefail

# shellcheck source=../../infra/trace/events.sh
source "$(dirname "${BASH_SOURCE[0]}")/../../infra/trace/events.sh" 2>/dev/null || true

lab_emit_event phase start

printf '\n\033[1;34m[claude-code-orchestrator-install]\033[0m no orchestrator-level install required\n'
lab_emit_event phase done
