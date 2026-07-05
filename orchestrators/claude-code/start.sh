#!/usr/bin/env bash
# Start Claude Code in a detached tmux session named "claude-code".
#
# If credentials are missing (no OAuth credentials),
# claude is still launched — it will prompt for login interactively.
# The browser OAuth flow works via Lima's transparent localhost forwarding.
#
# Attach to the session with: taxiway shell claude-code

set -euo pipefail

# shellcheck source=../../infra/trace/events.sh
source "$(dirname "${BASH_SOURCE[0]}")/../../infra/trace/events.sh" 2>/dev/null || true

if [ -f "${HOME}/.config/taxiway/env" ]; then
    set -a
    # shellcheck disable=SC1091
    . "${HOME}/.config/taxiway/env"
    set +a
fi

lab_emit_event phase start

log()  { printf '\n\033[1;34m[claude-code-start]\033[0m %s\n' "$*"; }
pass() { printf '  \033[1;32mOK\033[0m   %s\n' "$*"; }

SESSION="claude-code"
CLAUDE_CODE_MODEL="${TAXIWAY_SET_MODEL:-claude-opus-4-8}"
TAXIWAY_LITELLM_BASE_URL="${TAXIWAY_LITELLM_BASE_URL:-http://${TAXIWAY_LAB:-lab}.litellm.localhost:4000}"

if [ -z "${TAXIWAY_LITELLM_API_KEY:-}" ]; then
    printf '  \033[1;31mERROR\033[0m LiteLLM is required for Claude Code but TAXIWAY_LITELLM_API_KEY is missing\n'
    printf '        From the host, run: taxiway gateway %s\n' "${TAXIWAY_LAB:-<lab>}"
    exit 1
fi

export ANTHROPIC_BASE_URL="${TAXIWAY_LITELLM_BASE_URL%/}"
export ANTHROPIC_CUSTOM_HEADERS="x-litellm-api-key: Bearer ${TAXIWAY_LITELLM_API_KEY}"
export CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC="${CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC:-1}"

# Kill existing session if any (clean restart)
if tmux has-session -t "$SESSION" 2>/dev/null; then
    log "Killing existing tmux session '$SESSION'"
    tmux kill-session -t "$SESSION"
fi

# Auth status (informational only — not blocking)
log "Auth status"
if [ -f "${HOME}/.claude/.credentials.json" ]; then
    pass "OAuth credentials found at ~/.claude/.credentials.json"
else
    printf '  \033[1;33mWARN\033[0m No Claude Code OAuth credentials found\n'
    printf '       claude will prompt for login on first use\n'
    printf '       Attach with: taxiway shell claude-code\n'
fi

# Launch claude in a detached tmux session.
log "Starting tmux session '$SESSION'"
# Build env args explicitly. Existing tmux servers do not reliably inherit
# environment changes from this shell, so pass proxy/auth variables at session
# creation time.
tmux_env_args=()
for name in \
    ANTHROPIC_BASE_URL \
    ANTHROPIC_CUSTOM_HEADERS \
    CLAUDE_CODE_MODEL \
    CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC \
    TAXIWAY_LITELLM_BASE_URL \
    TAXIWAY_LITELLM_API_KEY
do
    if [ -n "${!name:-}" ]; then
        tmux_env_args+=(-e "${name}=${!name}")
    fi
done
# Use TAXIWAY_WORKSPACE_DIR as the working directory if set and exists.
# Without a cloned repo, start in /lab/work rather than $HOME so the session
# stays inside the lab workspace.
start_dir="/lab/work"
mkdir -p "$start_dir"
if [ -n "${TAXIWAY_WORKSPACE_DIR:-}" ] && [ -d "${TAXIWAY_WORKSPACE_DIR}" ]; then
    start_dir="${TAXIWAY_WORKSPACE_DIR}"
fi
agent_cmd="claude --model \"$CLAUDE_CODE_MODEL\""
tmux new-session -d -s "$SESSION" -c "$start_dir" "${tmux_env_args[@]}" "$agent_cmd"
pass "Claude Code started in tmux session '$SESSION' using model ${CLAUDE_CODE_MODEL}"
printf '  Attach with: taxiway shell claude-code\n'

lab_emit_event phase done
