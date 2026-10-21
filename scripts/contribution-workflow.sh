#!/usr/bin/env bash
# Thin client entry points; the canonical skill owns the workflow.
set -euo pipefail

usage() {
  cat <<'HELP'
Usage: scripts/contribution-workflow.sh <codex|claude> [settings] [-- client-options...]

Settings (positive integers, optional limits):
  --max-agents COUNT         Optional ceiling on concurrent agents, coordinator
                             included (default: sized to measured capacity)
  --max-heavy-tests COUNT    Concurrent Docker/Lima test runs (default: 1)

Starts an interactive session in this checkout with the same skill and intake.
No background runner is installed.
Client options are forwarded exactly, without changing permissions or login.
HELP
}

fail() { printf 'workflow: %s\n' "$*" >&2; exit 2; }
if [[ "${1:-}" == --help || "${1:-}" == -h ]]; then usage; exit 0; fi
[[ $# -gt 0 ]] || { usage >&2; exit 2; }
client="$1"
shift
case "$client" in codex|claude) ;; *) fail "client must be codex or claude" ;; esac
max_agents=
max_heavy_tests=1
while [[ $# -gt 0 ]]; do
  case "$1" in
    --) shift; break ;;
    --max-agents|--max-heavy-tests)
      [[ $# -ge 2 && "$2" =~ ^[1-9][0-9]*$ ]] || fail "$1 requires a positive integer"
      case "$1" in
        --max-agents) max_agents="$2" ;;
        --max-heavy-tests) max_heavy_tests="$2" ;;
      esac
      shift 2 ;;
    *) fail "unknown setting $1; put client options after --" ;;
  esac
done
repo_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
[[ -r "$repo_dir/.agents/skills/taxiway-workflow/SKILL.md" ]] || fail 'canonical skill is missing'
command -v "$client" >/dev/null 2>&1 || fail "$client is not installed"
cd "$repo_dir"
agents_limit="Size agent concurrency to measured host capacity"
[[ -z "$max_agents" ]] || agents_limit="$agents_limit, at most $max_agents agents including the coordinator"
prompt="Use the Taxiway contribution workflow. Read .agents/skills/taxiway-workflow/SKILL.md and AGENTS.md before acting, and follow the skill: it owns intake, approvals, coordination, status and resume. Start the intake dialogue: ask whether I have a new need or problem to analyze, or want suggestions from the repository backlog. Reconcile existing work before assigning anything. $agents_limit, and run at most $max_heavy_tests heavy Docker/Lima test runs at a time. Use one canonical workflow for both clients."
exec "$client" "$@" "$prompt"
