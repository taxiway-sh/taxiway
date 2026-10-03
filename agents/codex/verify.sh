#!/usr/bin/env bash
# Verify the installed `codex` CLI without making any API call.

set -euo pipefail

source "$(dirname "${BASH_SOURCE[0]}")/../../infra/trace/events.sh" 2>/dev/null || true
# shellcheck source=../../infra/commands/steps.sh
source "$(dirname "${BASH_SOURCE[0]}")/../../infra/commands/steps.sh"
# shellcheck source=../../infra/agents/npm-agent.sh
source "$(dirname "${BASH_SOURCE[0]}")/../../infra/agents/npm-agent.sh"

log()  { printf '\n\033[1;34m[codex-agent-verify]\033[0m %s\n' "$*"; }
pass() { printf '  \033[1;32mOK\033[0m   %s\n' "$*"; }
fail() { printf '  \033[1;31mFAIL\033[0m %s\n' "$*" >&2; exit 1; }

if taxiway_is_plan; then
  log "Verifying codex binary"
  if taxiway_can_inspect; then
    if command -v codex >/dev/null 2>&1; then
      taxiway_plan_detail "codex is present"
    else
      taxiway_plan_detail "codex is missing"
    fi
  else
    taxiway_plan_detail "codex"
  fi
  log "Verifying codex version and help"
  taxiway_plan_detail "codex --version"
  taxiway_plan_detail "codex --help"
  log "Checking Codex authentication configuration"
  taxiway_plan_detail "Taxiway LiteLLM gateway key"
  exit 0
fi

lab_emit_event phase start

CODEX="$(command -v codex || true)"
[ -n "$CODEX" ] || fail "codex not found - run: taxiway install <lab>"

log "binaries"
pass "codex binary at $CODEX"

log "codex --version"
version_out="$(mktemp -t codex-version.XXXXXX)"
"$CODEX" --version >"$version_out" 2>&1 \
  || { cat "$version_out" >&2; rm -f "$version_out"; fail "codex --version failed"; }
pass "$(tr '\n' ' ' <"$version_out")"
npm_agent_verify_version codex codex-version "${TAXIWAY_SET_CODEX_VERSION:-latest}" "$(awk 'NR == 1 { print $NF }' "$version_out")" "$CODEX"
rm -f "$version_out"

log "codex --help"
"$CODEX" --help >/dev/null 2>&1 || fail "codex --help failed"
pass "help OK"

log "Auth status check"
if [ -n "${TAXIWAY_LITELLM_API_KEY:-}" ]; then
  pass "LiteLLM gateway key is set - Codex will use Taxiway LiteLLM"
else
  log "LiteLLM gateway key is missing - from the host, run: taxiway gateway ${TAXIWAY_LAB:-<lab>}"
fi

log "Verify passed."
lab_emit_event phase done
