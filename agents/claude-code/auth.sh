#!/usr/bin/env bash
# Run Claude Code authentication interactively inside the lab.

set -euo pipefail

if [ -f "${HOME}/.config/taxiway/env" ]; then
    set -a
    # shellcheck disable=SC1091
    . "${HOME}/.config/taxiway/env"
    set +a
fi

log()  { printf '\n\033[1;34m[claude-code-auth]\033[0m %s\n' "$*"; }
pass() { printf '  \033[1;32mOK\033[0m   %s\n' "$*"; }
fail() { printf '  \033[1;31mFAIL\033[0m %s\n' "$*" >&2; exit 1; }

CLAUDE="$(command -v claude || true)"
[ -n "$CLAUDE" ] || fail "claude not found - run: taxiway install <lab>"

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
