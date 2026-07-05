#!/usr/bin/env bash
# Verify the installed Gas Town CLI without running any agent.
# Verifies:
#   1. `gt`, `bd`, `dolt`, and `sqlite3` are available
#   2. each binary reports a version

set -euo pipefail

log()  { printf '\n\033[1;34m[gastown-verify]\033[0m %s\n' "$*"; }
pass() { printf '  \033[1;32mOK\033[0m   %s\n' "$*"; }
fail() { printf '  \033[1;31mFAIL\033[0m %s\n' "$*" >&2; exit 1; }

export PATH="$HOME/.local/bin:$PATH"

find_bin() {
  local name="$1"
  local fallback="$2"
  local bin
  bin="$(command -v "$name" || true)"
  if [ -z "$bin" ] && [ -x "$fallback" ]; then
    bin="$fallback"
  fi
  [ -n "$bin" ] || fail "$name not found - run: make gastown-install"
  printf '%s\n' "$bin"
}

GT="$(find_bin gt "$HOME/.local/bin/gt")"
BD="$(find_bin bd "$HOME/.local/bin/bd")"
DOLT="$(find_bin dolt /usr/local/bin/dolt)"
SQLITE3="$(find_bin sqlite3 /usr/bin/sqlite3)"

log "binaries"
pass "gt binary at $GT"
pass "bd binary at $BD"
pass "dolt binary at $DOLT"
pass "sqlite3 binary at $SQLITE3"

log "versions"
"$GT" version >/tmp/gt-version 2>&1 || "$GT" --version >/tmp/gt-version 2>&1 || fail "gt version command failed"
pass "gt: $(tr '\n' ' ' </tmp/gt-version)"
rm -f /tmp/gt-version

"$BD" version >/tmp/bd-version 2>&1 || fail "bd version command failed"
pass "bd: $(tr '\n' ' ' </tmp/bd-version)"
rm -f /tmp/bd-version

"$DOLT" version >/tmp/dolt-version 2>&1 || fail "dolt version command failed"
pass "dolt: $(tr '\n' ' ' </tmp/dolt-version)"
rm -f /tmp/dolt-version

"$SQLITE3" --version >/tmp/sqlite3-version 2>&1 || fail "sqlite3 version command failed"
pass "sqlite3: $(tr '\n' ' ' </tmp/sqlite3-version)"
rm -f /tmp/sqlite3-version

log "Verify test passed."
