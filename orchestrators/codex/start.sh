#!/usr/bin/env bash
# Start Codex in a detached tmux session named "codex".
#
# Codex authentication is managed by LiteLLM; labs only need the LiteLLM key.
#
# Attach to the session with: taxiway shell codex

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

log()  { printf '\n\033[1;34m[codex-start]\033[0m %s\n' "$*"; }
pass() { printf '  \033[1;32mOK\033[0m   %s\n' "$*"; }

SESSION="codex"
CODEX_MODEL="${TAXIWAY_SET_MODEL:-gpt-5.5}"
TAXIWAY_LITELLM_BASE_URL="${TAXIWAY_LITELLM_BASE_URL:-http://${TAXIWAY_LAB:-lab}.litellm.localhost:4000}"
TAXIWAY_LITELLM_OPENAI_BASE_URL="${TAXIWAY_LITELLM_BASE_URL%/}/v1"
TAXIWAY_LITELLM_AGENT_ID="${TAXIWAY_LITELLM_AGENT_ID:-${TAXIWAY_AGENT:-codex}}"

# Use TAXIWAY_WORKSPACE_DIR as the working directory if set and exists.
# Without a cloned repo, start in /lab/work rather than $HOME so the session
# stays inside the lab workspace.
start_dir="/lab/work"
mkdir -p "$start_dir"
if [ -n "${TAXIWAY_WORKSPACE_DIR:-}" ] && [ -d "${TAXIWAY_WORKSPACE_DIR}" ]; then
    start_dir="${TAXIWAY_WORKSPACE_DIR}"
fi
trusted_project_dirs=("/lab/work")
if [ "$start_dir" != "/lab/work" ]; then
    trusted_project_dirs+=("$start_dir")
fi
project_headers=()
for trusted_project_dir in "${trusted_project_dirs[@]}"; do
    toml_trusted_project_dir="${trusted_project_dir//\\/\\\\}"
    toml_trusted_project_dir="${toml_trusted_project_dir//\"/\\\"}"
    project_headers+=("[projects.\"${toml_trusted_project_dir}\"]")
done

# Kill existing session if any (clean restart)
if tmux has-session -t "$SESSION" 2>/dev/null; then
    log "Killing existing tmux session '$SESSION'"
    tmux kill-session -t "$SESSION"
fi

# Auth status
log "Auth status"
if [ -z "${TAXIWAY_LITELLM_API_KEY:-}" ]; then
    printf '  \033[1;31mERROR\033[0m LiteLLM is required for Codex but TAXIWAY_LITELLM_API_KEY is missing\n'
    printf '        From the host, run: taxiway gateway %s\n' "${TAXIWAY_LAB:-<lab>}"
    exit 1
fi
pass "LiteLLM gateway key found"

log "LiteLLM provider"
mkdir -p "${HOME}/.codex"
CODEX_CONFIG="${HOME}/.codex/config.toml"
tmp_config="$(mktemp)"
{
    printf 'model_provider = "taxiway-litellm"\n'
    printf 'model = "%s"\n' "$CODEX_MODEL"
    printf '\n'
} > "$tmp_config"
if [ -f "$CODEX_CONFIG" ]; then
    awk -v project_headers="$(printf '%s\n' "${project_headers[@]}")" '
        BEGIN {
            split(project_headers, headers, "\n")
            for (i in headers) {
                trusted_headers[headers[i]] = 1
            }
        }
        /^\[model_providers\.taxiway-litellm\]$/ { skip=1; next }
        $0 in trusted_headers { skip=1; next }
        /^\[/ { skip=0 }
        skip { next }
        /^model_provider = / { next }
        /^model = / { next }
        { print }
    ' "$CODEX_CONFIG" >> "$tmp_config"
fi
cat >> "$tmp_config" << EOF

[model_providers.taxiway-litellm]
name = "Taxiway LiteLLM"
base_url = "${TAXIWAY_LITELLM_OPENAI_BASE_URL}"
wire_api = "responses"
requires_openai_auth = false
env_http_headers = { "x-litellm-api-key" = "TAXIWAY_LITELLM_API_KEY", "x-litellm-agent-id" = "TAXIWAY_LITELLM_AGENT_ID" }
supports_websockets = false
EOF
for project_header in "${project_headers[@]}"; do
    cat >> "$tmp_config" << EOF

${project_header}
trust_level = "trusted"
EOF
done
mv "$tmp_config" "$CODEX_CONFIG"
pass "Codex configured for LiteLLM subscription proxy using model ${CODEX_MODEL}"

# Launch codex in a detached tmux session
log "Starting tmux session '$SESSION'"
# Build env args explicitly. Existing tmux servers do not reliably inherit
# environment changes from this shell, so pass proxy variables at session
# creation time.
tmux_env_args=()
for name in \
    TAXIWAY_LITELLM_API_KEY \
    TAXIWAY_LITELLM_BASE_URL \
    TAXIWAY_LITELLM_AGENT_ID
do
    if [ -n "${!name:-}" ]; then
        tmux_env_args+=(-e "${name}=${!name}")
    fi
done
agent_cmd="codex resume --last || codex"
tmux new-session -d -s "$SESSION" -c "$start_dir" "${tmux_env_args[@]}" "$agent_cmd"
pass "Codex started in tmux session '$SESSION'"
printf '  Attach with: taxiway shell codex\n'

lab_emit_event phase done
