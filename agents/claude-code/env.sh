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
    } > "$settings_tmp"
    mv -f "$settings_tmp" "$settings_dir/claude-code.env"
)

claude_code_load_env() {
    if [[ -f "$HOME/.config/taxiway/agents/claude-code.env" ]]; then
        # shellcheck disable=SC1091
        source "$HOME/.config/taxiway/agents/claude-code.env"
    fi
}
