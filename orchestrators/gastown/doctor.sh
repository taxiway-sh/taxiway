#!/usr/bin/env bash
# Diagnose Gas Town orchestrator installation.

set -euo pipefail

export PATH="$HOME/.local/bin:$PATH"

"$(dirname "${BASH_SOURCE[0]}")/verify.sh"

HQ_DIR="${TAXIWAY_HQ_DIR:-/lab/work/gt}"
if [[ ! -d "$HQ_DIR" ]]; then
    echo "Gas Town HQ not found at $HQ_DIR; run taxiway start first" >&2
    exit 1
fi

cd "$HQ_DIR"
if [[ "${TAXIWAY_DOCTOR_FIX:-false}" == "true" ]]; then
    exec gt doctor --fix
fi

exec gt doctor
