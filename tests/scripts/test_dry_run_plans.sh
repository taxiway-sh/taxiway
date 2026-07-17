#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
TEST_ROOT="$(mktemp -d)"
trap 'rm -rf "$TEST_ROOT"' EXIT

SAFE_BIN="$TEST_ROOT/bin"
TEST_HOME="$TEST_ROOT/home"
FORBIDDEN_LOG="$TEST_ROOT/forbidden.log"
mkdir -p "$SAFE_BIN" "$TEST_HOME"
export FORBIDDEN_LOG

for command_name in sudo apt-get curl npm gt bd dolt sqlite3 codex claude git tmux docker; do
  printf '%s\n' \
    '#!/usr/bin/env bash' \
    'printf "%s\\n" "$(basename "$0") $*" >> "$FORBIDDEN_LOG"' \
    'exit 97' >"$SAFE_BIN/$command_name"
  chmod +x "$SAFE_BIN/$command_name"
done

run_plan() {
  local script="$1"
  shift
  : > "$FORBIDDEN_LOG"
  TAXIWAY_EXECUTION_MODE=plan \
  TAXIWAY_PLAN_INSPECTION=unavailable \
  HOME="$TEST_HOME" \
  PATH="$SAFE_BIN:$PATH" \
  "$@" bash "$script"
  [[ ! -s "$FORBIDDEN_LOG" ]]
}

assert_contains() {
  local output="$1"
  local expected="$2"
  if [[ "$output" != *"$expected"* ]]; then
    printf 'missing expected dry-run label: %s\noutput:\n%s\n' "$expected" "$output" >&2
    return 1
  fi
}

output="$(run_plan "$ROOT_DIR/agents/codex/install.sh")"
assert_contains "$output" "[codex-agent-install]"
assert_contains "$output" "Installing bubblewrap (codex sandbox prerequisite)"
assert_contains "$output" "Installing @openai/codex@latest"

output="$(run_plan "$ROOT_DIR/agents/claude-code/install.sh")"
assert_contains "$output" "[claude-code-agent-install]"
assert_contains "$output" "Installing @anthropic-ai/claude-code@latest"

output="$(run_plan "$ROOT_DIR/orchestrators/gastown/install.sh")"
assert_contains "$output" "[gastown-install]"
assert_contains "$output" "Installing apt dependencies"
assert_contains "$output" "Installing Dolt"
assert_contains "$output" "Installing Gas Town"
assert_contains "$output" "Installing Beads"

output="$(run_plan "$ROOT_DIR/agents/codex/verify.sh")"
assert_contains "$output" "[codex-agent-verify]"
assert_contains "$output" "Verifying codex binary"
assert_contains "$output" "Verifying codex version and help"

output="$(run_plan "$ROOT_DIR/agents/claude-code/verify.sh")"
assert_contains "$output" "[claude-code-agent-verify]"
assert_contains "$output" "Verifying claude binary"
assert_contains "$output" "Verifying claude version, help, and auth status"

output="$(run_plan "$ROOT_DIR/orchestrators/gastown/verify.sh")"
assert_contains "$output" "[gastown-verify]"
assert_contains "$output" "Verifying required binaries"
assert_contains "$output" "Verifying tool versions"

printf 'dry-run plan scripts: OK\n'
