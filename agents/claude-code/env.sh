#!/usr/bin/env bash
# The orchestrator resolves its settings; this helper only stores and reloads
# the resulting environment for Claude Code launches and handoffs.

# claude_code_version_pinned <claude-code-version>
# A pinned Claude Code must not replace itself with another release.
claude_code_version_pinned() {
    [[ -n "${1:-}" && "$1" != "latest" ]]
}

# The common environment is already sourced by guest login shells. Keep the
# agent's update policy in its own managed block, preserving gateway settings.
claude_code_write_update_policy() {
    local pinned=false
    if claude_code_version_pinned "${TAXIWAY_SET_CLAUDE_CODE_VERSION:-}"; then
        pinned=true
    fi
    python3 - "$pinned" <<'POLICY_PY'
import os, pathlib, re, sys, tempfile
path = pathlib.Path.home() / '.config/taxiway/env'
path.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
os.chmod(path.parent, 0o700)
start = '# >>> taxiway agent-version scope=claude-code'
end = '# <<< taxiway agent-version scope=claude-code'
existing = path.read_text() if path.exists() else ''
existing = re.sub(r'(?m)^' + re.escape(start) + r'\n.*?^' + re.escape(end) + r'\n?', '', existing, flags=re.S).rstrip('\n')
block = start + '\nDISABLE_AUTOUPDATER=1\n' + end if sys.argv[1] == 'true' else ''
content = '\n\n'.join(part for part in (existing, block) if part) + '\n'
fd, name = tempfile.mkstemp(prefix='.version-policy-', dir=path.parent)
try:
    with os.fdopen(fd, 'w') as output:
        output.write(content)
    os.chmod(name, 0o600)
    os.replace(name, path)
finally:
    if os.path.exists(name): os.unlink(name)
POLICY_PY
}

claude_code_write_env() (
    set -euo pipefail
    umask 077
    claude_code_write_update_policy
    settings_dir="$HOME/.config/taxiway/agents"
    mkdir -p "$settings_dir"
    chmod 700 "$settings_dir"
    settings_tmp="$(mktemp "$settings_dir/.env.XXXXXX")"
    trap 'rm -f "$settings_tmp"' EXIT
    {
        printf '# Managed by Taxiway. Change with --set / --clear-set.\n'
        printf 'export ENABLE_TOOL_SEARCH=%q\n' "$1"
        printf 'export ENABLE_CLAUDEAI_MCP_SERVERS=%q\n' "$2"
        printf 'export TAXIWAY_CLAUDE_AUTH_MODE=%q\n' "${TAXIWAY_SET_AUTH_MODE:-subscription}"
        for name in ANTHROPIC_DEFAULT_OPUS_MODEL ANTHROPIC_DEFAULT_SONNET_MODEL ANTHROPIC_DEFAULT_HAIKU_MODEL ANTHROPIC_DEFAULT_FABLE_MODEL; do
            if [[ -n "${!name:-}" ]]; then
                printf 'export %s=%q\n' "$name" "${!name}"
            fi
        done
        if claude_code_version_pinned "${TAXIWAY_SET_CLAUDE_CODE_VERSION:-}"; then
            printf 'export TAXIWAY_PINNED_CLAUDE_CODE_VERSION=%q\n' "$TAXIWAY_SET_CLAUDE_CODE_VERSION"
            printf 'export DISABLE_AUTOUPDATER=1\n'
        fi
    } > "$settings_tmp"
    mv -f "$settings_tmp" "$settings_dir/claude-code.env"
    if [[ -n "${TAXIWAY_CLAUDE_AVAILABLE_MODELS:-}" ]]; then
        policy_path="${3:-/etc/claude-code/managed-settings.json}"
        policy_tmp="$(mktemp "$settings_dir/.policy.XXXXXX")"
        trap 'rm -f "$settings_tmp" "$policy_tmp"' EXIT
        python3 - "$policy_path" "$policy_tmp" <<'PY'
import json
import os
from pathlib import Path
import re
import subprocess
import sys

models = json.loads(os.environ["TAXIWAY_CLAUDE_AVAILABLE_MODELS"])
if not isinstance(models, list) or not models or not all(isinstance(m, str) and m for m in models):
    raise ValueError("Taxiway Claude model list must contain model IDs")
version = subprocess.check_output(["claude", "--version"], text=True)
match = re.search(r"(\d+)\.(\d+)\.(\d+)", version)
if not match or tuple(map(int, match.groups())) < (2, 1, 284):
    raise RuntimeError("Taxiway gateway models require Claude Code 2.1.284 or newer; rerun the install phase")
path = Path(sys.argv[1])
settings = json.loads(path.read_text()) if path.exists() else {}
settings["availableModels"] = models
settings["enforceAvailableModels"] = True
settings["availableModelsMatch"] = "exact"
with open(sys.argv[2], "w") as output:
    json.dump(settings, output, indent=2)
    output.write("\n")
PY
        if [[ "$policy_path" == /etc/claude-code/managed-settings.json ]]; then
            sudo install -d -m 0755 /etc/claude-code
            sudo install -m 0644 "$policy_tmp" "$policy_path"
        else
            install -d -m 0755 "$(dirname "$policy_path")"
            install -m 0644 "$policy_tmp" "$policy_path"
        fi
    fi
)

claude_code_load_env() {
    if [[ -f "$HOME/.config/taxiway/agents/claude-code.env" ]]; then
        unset TAXIWAY_PINNED_CLAUDE_CODE_VERSION DISABLE_AUTOUPDATER
        # shellcheck disable=SC1091
        source "$HOME/.config/taxiway/agents/claude-code.env"
        # Native gateway credentials replace OAuth only in the selected API
        # mode. A mode change must also clear a stale token inherited by tmux.
        if [[ "${TAXIWAY_CLAUDE_AUTH_MODE:-subscription}" == api-key ]]; then
            export ANTHROPIC_AUTH_TOKEN="${TAXIWAY_LITELLM_API_KEY:?Missing Taxiway gateway key}"
        else
            unset ANTHROPIC_AUTH_TOKEN
        fi
        if [[ -n "${TAXIWAY_PINNED_CLAUDE_CODE_VERSION:-}" ]]; then
            source "$(dirname "${BASH_SOURCE[0]}")/../../infra/agents/npm-agent.sh"
            npm_agent_verify_version claude-code claude-code-version "$TAXIWAY_PINNED_CLAUDE_CODE_VERSION" "$(claude --version | awk 'NR == 1 { print $1 }')" "$(command -v claude)" || return 1
        fi
    fi
}
