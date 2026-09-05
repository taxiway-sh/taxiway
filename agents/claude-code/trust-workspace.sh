#!/usr/bin/env bash
# Mark a Taxiway-managed workspace as trusted by Claude Code.

set -euo pipefail

workspace_path="${TAXIWAY_WORKSPACE_TRUST_PATH:-}"
if [[ -z "$workspace_path" || "$workspace_path" != /* ]]; then
    echo "TAXIWAY_WORKSPACE_TRUST_PATH must be an absolute path" >&2
    exit 1
fi
case "$workspace_path" in
    *$'\n'*|*$'\r'*)
        echo "TAXIWAY_WORKSPACE_TRUST_PATH must not contain newlines" >&2
        exit 1
        ;;
esac

config_path="${HOME}/.claude.json"
mkdir -p "$(dirname "$config_path")"
tmp_config="$(mktemp "${config_path}.tmp.XXXXXX")"
trap 'rm -f "$tmp_config"' EXIT

if [[ -f "$config_path" ]]; then
    jq --arg path "$workspace_path" '
        .projects = (.projects // {})
        | .projects[$path] = ((.projects[$path] // {}) + {hasTrustDialogAccepted: true})
    ' "$config_path" > "$tmp_config"
else
    jq -n --arg path "$workspace_path" '
        {projects: {($path): {hasTrustDialogAccepted: true}}}
    ' > "$tmp_config"
fi

chmod 0600 "$tmp_config"
mv "$tmp_config" "$config_path"
trap - EXIT
