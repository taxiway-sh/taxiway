#!/usr/bin/env bash
# Verify the installed `claude` CLI without making any API call.

set -euo pipefail

source "$(dirname "${BASH_SOURCE[0]}")/../../infra/trace/events.sh" 2>/dev/null || true

lab_emit_event phase start

log()  { printf '\n\033[1;34m[claude-code-agent-verify]\033[0m %s\n' "$*"; }
pass() { printf '  \033[1;32mOK\033[0m   %s\n' "$*"; }
fail() { printf '  \033[1;31mFAIL\033[0m %s\n' "$*" >&2; exit 1; }

CLAUDE="$(command -v claude || true)"
[ -n "$CLAUDE" ] || fail "claude not found - run: taxiway install <lab>"

log "binaries"
pass "claude binary at $CLAUDE"

log "claude --version"
cc_version="$(mktemp -t cc-version.XXXXXX)"
"$CLAUDE" --version >"$cc_version" 2>&1 || { cat "$cc_version" >&2; rm -f "$cc_version"; fail "claude --version failed"; }
pass "$(tr '\n' ' ' <"$cc_version")"
rm -f "$cc_version"

log "claude --help"
"$CLAUDE" --help >/dev/null 2>&1 || fail "claude --help failed"
pass "help OK"

log "claude auth status"
cc_config="$(mktemp -t cc-config.XXXXXX)"
set +e
env -u ANTHROPIC_API_KEY "$CLAUDE" config list >"$cc_config" 2>&1
exit_code=$?
set -e

if [ $exit_code -eq 0 ]; then
  pass "config readable (logged in or API key set)"
elif grep -qi "not logged in\|please run.*login\|unauthenticated" "$cc_config"; then
  pass "claude installed (not yet authenticated)"
else
  cat "$cc_config" >&2
  rm -f "$cc_config"
  fail "claude config list failed unexpectedly"
fi
rm -f "$cc_config"

log "Auth status check"
if [ -n "${ANTHROPIC_API_KEY:-}" ]; then
  pass "ANTHROPIC_API_KEY is set - API key auth will be used"
elif [ -f "${HOME}/.claude/.credentials.json" ]; then
  pass "OAuth credentials found at ~/.claude/.credentials.json"
else
  pass "No credentials found - run: taxiway auth <lab> claude-code (or set ANTHROPIC_API_KEY)"
fi

log "Verify passed."
lab_emit_event phase done
