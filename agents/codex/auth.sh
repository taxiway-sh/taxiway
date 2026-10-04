#!/usr/bin/env bash
# Validate Codex authentication before start.

set -euo pipefail

# shellcheck source=../../infra/commands/steps.sh
source "$(dirname "${BASH_SOURCE[0]}")/../../infra/commands/steps.sh"

if [ -f "${HOME}/.config/taxiway/env" ]; then
    set -a
    # shellcheck disable=SC1091
    . "${HOME}/.config/taxiway/env"
    set +a
fi

log()  { printf '\n\033[1;34m[codex-auth]\033[0m %s\n' "$*"; }
pass() { printf '  \033[1;32mOK\033[0m   %s\n' "$*"; }
fail() { printf '  \033[1;31mFAIL\033[0m %s\n' "$*" >&2; exit 1; }

if taxiway_is_plan; then
    log "Authentication is managed by the Taxiway LiteLLM gateway"
    if [[ -n "${TAXIWAY_LITELLM_API_KEY:-}" ]]; then
        taxiway_plan_detail "LiteLLM gateway key is available"
    else
        taxiway_plan_detail "LiteLLM gateway key would be required before start"
    fi
    exit 0
fi

CODEX="$(command -v codex || true)"
[ -n "$CODEX" ] || fail "codex not found - run: taxiway install <lab>"

if [ -n "${TAXIWAY_LITELLM_API_KEY:-}" ]; then
    pass "LiteLLM gateway key found - Codex auth is managed by Taxiway"
    exit 0
fi

fail "TAXIWAY_LITELLM_API_KEY is missing - from the host, run: taxiway gateway ${TAXIWAY_LAB:-<lab>}"
