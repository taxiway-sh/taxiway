#!/usr/bin/env bash
# Trust a Gas Town agent's workspace before replacing this launcher with it.
set -euo pipefail

if (( $# < 2 )) || [[ -z "$2" || "$2" == -* ]]; then
    echo "Usage: launch-agent.sh trust-root command [args...]" >&2
    exit 1
fi

# Gastown preserves agent arguments across handoff, but not arbitrary env vars.
trust_root="$1"
shift
if [[ "$trust_root" != /* || "$trust_root" == / ]]; then
    echo "Trust root must be an absolute workspace directory other than /" >&2
    exit 1
fi
trust_root="$(cd -- "$trust_root" && pwd -P)"
workspace_path="$(pwd -P)"
if [[ "$trust_root" == / || ( "$workspace_path" != "$trust_root" && "$workspace_path" != "$trust_root/"* ) ]]; then
    echo "Current directory is outside the configured trust root" >&2
    exit 1
fi

# A handoff also drops the profile's gateway environment. Reload the managed
# Lab configuration instead of putting credentials into the command arguments.
if [[ -f "$HOME/.config/taxiway/env" ]]; then
    set -a
    # shellcheck disable=SC1091
    source "$HOME/.config/taxiway/env"
    set +a
fi
: "${TAXIWAY_LITELLM_BASE_URL:?Missing Taxiway gateway URL}"
: "${TAXIWAY_LITELLM_API_KEY:?Missing Taxiway gateway key}"
export ANTHROPIC_BASE_URL="${TAXIWAY_LITELLM_BASE_URL%/}"
export ANTHROPIC_CUSTOM_HEADERS="x-litellm-api-key: Bearer ${TAXIWAY_LITELLM_API_KEY}"
export CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC="${CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC:-1}"

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
TAXIWAY_WORKSPACE_TRUST_PATH="$workspace_path" \
    bash "$script_dir/../../agents/claude-code/trust-workspace.sh"
exec "$@"
