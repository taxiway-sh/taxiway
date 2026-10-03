#!/usr/bin/env bash
# Install the Anthropic `claude` CLI (npm package @anthropic-ai/claude-code).
#
# The version comes from the orchestrator setting claude-code-version:
# omitted or "latest" keeps an existing installation, an exact release is
# installed (upgrade or downgrade).

set -euo pipefail

source "$(dirname "${BASH_SOURCE[0]}")/../../infra/trace/events.sh" 2>/dev/null || true
# shellcheck source=../../infra/commands/steps.sh
source "$(dirname "${BASH_SOURCE[0]}")/../../infra/commands/steps.sh"
# shellcheck source=../../infra/agents/npm-agent.sh
source "$(dirname "${BASH_SOURCE[0]}")/../../infra/agents/npm-agent.sh"

log() { printf '\n\033[1;34m[claude-code-agent-install]\033[0m %s\n' "$*"; }

# `claude --version` prints "X.Y.Z (Claude Code)".
claude_version() {
  command -v claude >/dev/null 2>&1 || return 0
  claude --version 2>/dev/null | awk 'NR == 1 { print $1 }' || true
}

PKG="@anthropic-ai/claude-code"
VERSION="${TAXIWAY_SET_CLAUDE_CODE_VERSION:-latest}"

if taxiway_is_plan; then
  if taxiway_can_inspect && command -v claude >/dev/null 2>&1; then
    log "Checking the installed claude version before installing ${PKG}@${VERSION}"
  else
    log "Installing ${PKG}@${VERSION}"
  fi
  exit 0
fi

lab_emit_event phase start

command -v npm >/dev/null 2>&1 || { echo "npm missing - run taxiway bootstrap first" >&2; exit 1; }

npm_agent_install claude-code claude-code-version @anthropic-ai/claude-code \
  "${TAXIWAY_SET_CLAUDE_CODE_VERSION:-latest}" claude_version

lab_emit_event phase done
