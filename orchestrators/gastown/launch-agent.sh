#!/usr/bin/env bash
# Trust a Gas Town agent's workspace before replacing this launcher with it.
set -euo pipefail

if (( $# == 0 )) || [[ -z "$1" || "$1" == -* ]]; then
    echo "Usage: launch-agent.sh command [args...]" >&2
    exit 1
fi

trust_root="${TAXIWAY_WORKSPACE_TRUST_ROOT:-}"
if [[ "$trust_root" != /* || "$trust_root" == / ]]; then
    echo "TAXIWAY_WORKSPACE_TRUST_ROOT must be an absolute workspace directory" >&2
    exit 1
fi
trust_root="$(cd -- "$trust_root" && pwd -P)"
workspace_path="$(pwd -P)"
if [[ "$trust_root" == / || ( "$workspace_path" != "$trust_root" && "$workspace_path" != "$trust_root/"* ) ]]; then
    echo "Current directory is outside TAXIWAY_WORKSPACE_TRUST_ROOT" >&2
    exit 1
fi

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
TAXIWAY_WORKSPACE_TRUST_PATH="$workspace_path" \
    bash "$script_dir/../../agents/claude-code/trust-workspace.sh"
exec "$@"
