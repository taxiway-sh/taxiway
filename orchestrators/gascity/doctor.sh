#!/usr/bin/env bash
# Diagnose the Gas City orchestrator installation and city.

set -euo pipefail

export PATH="$HOME/.local/bin:$PATH"

"$(dirname "${BASH_SOURCE[0]}")/verify.sh"

CITY_DIR="${TAXIWAY_CITY_DIR:-/lab/work/gc}"
if [[ ! -f "$CITY_DIR/city.toml" ]]; then
    echo "Gas City city not found at $CITY_DIR; run taxiway start first" >&2
    exit 1
fi

cd "$CITY_DIR"
if [[ "${TAXIWAY_DOCTOR_FIX:-false}" == "true" ]]; then
    exec gc doctor --fix
fi

exec gc doctor
