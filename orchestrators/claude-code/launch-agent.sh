#!/usr/bin/env bash
set -euo pipefail
# Read settings inside the tmux session to override its inherited environment.
# shellcheck source=../../agents/claude-code/env.sh
source "$(dirname "${BASH_SOURCE[0]}")/../../agents/claude-code/env.sh"
claude_code_load_env
# This adapter requires autonomous execution, including reconstructed handoffs.
# shellcheck source=../../agents/claude-code/permissions.sh
source "$(dirname "${BASH_SOURCE[0]}")/../../agents/claude-code/permissions.sh"
claude_code_exec_autonomous "$@"
