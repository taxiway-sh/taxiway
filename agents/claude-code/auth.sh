#!/usr/bin/env bash
# Run Claude Code authentication interactively inside the lab.

set -euo pipefail

# shellcheck source=../../infra/commands/steps.sh
source "$(dirname "${BASH_SOURCE[0]}")/../../infra/commands/steps.sh"

if [ -f "${HOME}/.config/taxiway/env" ]; then
    set -a
    # shellcheck disable=SC1091
    . "${HOME}/.config/taxiway/env"
    set +a
fi

log()  { printf '\n\033[1;34m[claude-code-auth]\033[0m %s\n' "$*"; }
pass() { printf '  \033[1;32mOK\033[0m   %s\n' "$*"; }
fail() { printf '  \033[1;31mFAIL\033[0m %s\n' "$*" >&2; exit 1; }

if taxiway_is_plan; then
    log "Checking Claude Code authentication"
    if [[ "${TAXIWAY_AUTH_MODE:-subscription}" == "api-key" ]]; then
        taxiway_plan_detail "Authentication is managed by the Taxiway LiteLLM gateway"
    elif taxiway_can_inspect && [[ -s "${HOME}/.claude/.credentials.json" ]]; then
        taxiway_plan_detail "OAuth credentials are available"
    else
        log "Starting Claude Code interactive authentication if credentials are missing"
    fi
    exit 0
fi

CLAUDE="$(command -v claude || true)"
[ -n "$CLAUDE" ] || fail "claude not found - run: taxiway install <lab>"

# shellcheck source=env.sh
source "$(dirname "${BASH_SOURCE[0]}")/env.sh"
if claude_code_version_pinned "${TAXIWAY_SET_CLAUDE_CODE_VERSION:-}"; then
    export DISABLE_AUTOUPDATER=1
    source "$(dirname "${BASH_SOURCE[0]}")/../../infra/agents/npm-agent.sh"
    npm_agent_verify_version claude-code claude-code-version "$TAXIWAY_SET_CLAUDE_CODE_VERSION" "$("$CLAUDE" --version | awk 'NR == 1 { print $1 }')" "$CLAUDE"
fi

case "${TAXIWAY_AUTH_MODE:-subscription}" in
  api-key)
    if [ -n "${TAXIWAY_LITELLM_API_KEY:-}" ]; then
        pass "LiteLLM gateway key found - provider API key is managed by LiteLLM"
        exit 0
    fi
    fail "TAXIWAY_LITELLM_API_KEY is missing - from the host, run: taxiway gateway ${TAXIWAY_LAB:-<lab>}"
    ;;
esac

if [ -s "${HOME}/.claude/.credentials.json" ]; then
    pass "OAuth credentials found at ~/.claude/.credentials.json"
    exit 0
fi

log "Starting Claude Code interactive authentication"
printf 'Complete the Claude Code login flow below. When finished, exit Claude Code to continue with taxiway.\n\n'

"$CLAUDE"

if [ -s "${HOME}/.claude/.credentials.json" ]; then
    pass "OAuth credentials found at ~/.claude/.credentials.json"
else
    fail "Claude Code exited without writing ~/.claude/.credentials.json"
fi
