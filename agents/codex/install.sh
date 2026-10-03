#!/usr/bin/env bash
# Install the OpenAI `codex` CLI (npm package @openai/codex).
#
# The version comes from the orchestrator setting codex-version: omitted or
# "latest" keeps an existing installation, an exact release is installed
# (upgrade or downgrade).

set -euo pipefail

source "$(dirname "${BASH_SOURCE[0]}")/../../infra/trace/events.sh" 2>/dev/null || true
# shellcheck source=../../infra/commands/steps.sh
source "$(dirname "${BASH_SOURCE[0]}")/../../infra/commands/steps.sh"
# shellcheck source=../../infra/agents/npm-agent.sh
source "$(dirname "${BASH_SOURCE[0]}")/../../infra/agents/npm-agent.sh"

log() { printf '\n\033[1;34m[codex-agent-install]\033[0m %s\n' "$*"; }

# `codex --version` prints "codex-cli X.Y.Z".
codex_version() {
  command -v codex >/dev/null 2>&1 || return 0
  codex --version 2>/dev/null | awk 'NR == 1 { print $NF }' || true
}

PKG="@openai/codex"
VERSION="${TAXIWAY_SET_CODEX_VERSION:-latest}"

if taxiway_is_plan; then
  if taxiway_can_inspect && command -v bwrap >/dev/null 2>&1; then
    log "bubblewrap already installed"
  else
    log "Installing bubblewrap (codex sandbox prerequisite)"
  fi

  if taxiway_can_inspect && command -v codex >/dev/null 2>&1; then
    log "Checking the installed codex version before installing ${PKG}@${VERSION}"
  else
    log "Installing ${PKG}@${VERSION}"
  fi
  exit 0
fi

lab_emit_event phase start

command -v npm >/dev/null 2>&1 || { echo "npm missing - run taxiway bootstrap first" >&2; exit 1; }

if ! command -v bwrap >/dev/null 2>&1; then
  log "Installing bubblewrap (codex sandbox prerequisite)"
  sudo apt-get install -y --no-install-recommends bubblewrap
else
  log "bubblewrap already installed: $(bwrap --version 2>/dev/null || echo 'ok')"
fi

npm_agent_install codex codex-version @openai/codex \
  "${TAXIWAY_SET_CODEX_VERSION:-latest}" codex_version

lab_emit_event phase done
