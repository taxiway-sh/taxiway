#!/usr/bin/env bash
# Diagnose the Codex agent installation and auth state.

set -euo pipefail

exec "$(dirname "${BASH_SOURCE[0]}")/verify.sh"
