#!/usr/bin/env bash
# The orchestrator resolves its settings; this helper only stores and reloads
# the resulting environment for Claude Code launches and handoffs.
claude_code_write_env() (
    set -euo pipefail
    umask 077
    settings_dir="$HOME/.config/taxiway/agents"
    mkdir -p "$settings_dir"
    chmod 700 "$settings_dir"
    settings_tmp="$(mktemp "$settings_dir/.env.XXXXXX")"
    trap 'rm -f "$settings_tmp"' EXIT
    {
        printf '# Managed by Taxiway. Change with --set / --clear-set.\n'
        printf 'export ENABLE_TOOL_SEARCH=%q\n' "$1"
        printf 'export ENABLE_CLAUDEAI_MCP_SERVERS=%q\n' "$2"
        for name in ANTHROPIC_DEFAULT_OPUS_MODEL ANTHROPIC_DEFAULT_SONNET_MODEL ANTHROPIC_DEFAULT_HAIKU_MODEL ANTHROPIC_DEFAULT_FABLE_MODEL; do
            if [[ -n "${!name:-}" ]]; then
                printf 'export %s=%q\n' "$name" "${!name}"
            fi
        done
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
        # shellcheck disable=SC1091
        source "$HOME/.config/taxiway/agents/claude-code.env"
    fi
}
