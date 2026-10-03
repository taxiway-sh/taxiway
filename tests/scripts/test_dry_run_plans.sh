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
export CODEX_VERSION=9.8.7 CLAUDE_CODE_VERSION=9.8.7

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
  if [[ -n "$(find "$TEST_HOME" -mindepth 1 -print -quit)" ]]; then
    printf 'dry-run changed temporary HOME in %s\n' "$script" >&2
    return 1
  fi
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
assert_contains "$output" "Installing @openai/codex@9.8.7"

output="$(run_plan "$ROOT_DIR/agents/claude-code/install.sh")"
assert_contains "$output" "[claude-code-agent-install]"
assert_contains "$output" "Installing @anthropic-ai/claude-code@9.8.7"

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

output="$(run_plan "$ROOT_DIR/orchestrators/codex/workspace.sh" env \
  TAXIWAY_REPO_URL=https://github.com/acme/project.git \
  TAXIWAY_WORKSPACE_DIR=/lab/work/project \
  TAXIWAY_REPO_REF=main)"
assert_contains "$output" "Preparing local repository mirror"
assert_contains "$output" "Cloning workspace repository"
assert_contains "$output" "Checking out workspace ref main"

output="$(run_plan "$ROOT_DIR/orchestrators/gastown/workspace.sh" env \
  TAXIWAY_REPO_URL=https://github.com/acme/project.git \
  TAXIWAY_RIG_NAME=project \
  TAXIWAY_CREW_NAME=developer \
  TAXIWAY_REPO_REF=main)"
assert_contains "$output" "Initializing Gas Town HQ"
assert_contains "$output" "Adding rig 'project'"
assert_contains "$output" "Checking out rig ref main"
assert_contains "$output" "Adding crew workspace 'developer'"

output="$(run_plan "$ROOT_DIR/agents/codex/auth.sh")"
assert_contains "$output" "Authentication is managed by the Taxiway LiteLLM gateway"

output="$(run_plan "$ROOT_DIR/agents/claude-code/auth.sh")"
assert_contains "$output" "Checking Claude Code authentication"
assert_contains "$output" "Starting Claude Code interactive authentication if credentials are missing"

output="$(run_plan "$ROOT_DIR/orchestrators/codex/start.sh" env TAXIWAY_LAB=demo TAXIWAY_LITELLM_API_KEY=test TAXIWAY_SET_MODEL=test-model)"
assert_contains "$output" "Configuring Codex for the Taxiway LiteLLM gateway"
assert_contains "$output" "Starting tmux session 'codex'"

output="$(run_plan "$ROOT_DIR/orchestrators/claude-code/start.sh" env TAXIWAY_LAB=demo TAXIWAY_LITELLM_API_KEY=test TAXIWAY_SET_MODEL=test-model)"
assert_contains "$output" "Configuring Claude Code for the Taxiway LiteLLM gateway"
assert_contains "$output" "Starting tmux session 'claude-code'"

output="$(run_plan "$ROOT_DIR/orchestrators/gastown/start.sh" env \
  TAXIWAY_LAB=demo \
  TAXIWAY_LITELLM_API_KEY=test TAXIWAY_SET_MODEL=test-model \
  TAXIWAY_RIG_NAME=project \
  TAXIWAY_CREW_NAME=developer)"
assert_contains "$output" "Checking Gas Town runtime health"
assert_contains "$output" "Starting Gas Town daemon if required"
assert_contains "$output" "Starting crew workspace"
assert_contains "$output" "Starting tmux session 'gastown'"

output="$(run_plan "$ROOT_DIR/infra/commands/reset.sh")"
assert_contains "$output" "Stopping workspace services"
assert_contains "$output" "Clearing /lab/work contents"

printf 'dry-run plan scripts: OK\n'
