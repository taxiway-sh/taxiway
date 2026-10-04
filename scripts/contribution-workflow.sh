#!/usr/bin/env bash
# Thin client entry points; the canonical skill owns the workflow.
set -euo pipefail

usage() {
  cat <<'HELP'
Usage: scripts/contribution-workflow.sh <codex|claude> [settings] [-- client-options...]

Settings (positive integers):
  --status-interval SECONDS   Overall status cadence (default: 300)
  --triage-interval SECONDS   Backlog proposal cadence (default: 900)
  --max-agents COUNT         Concurrent agents, coordinator included (default: 2)
  --max-heavy-tests COUNT    Concurrent Docker/Lima test runs (default: 1)

Starts an interactive session in this checkout with the same skill and intake.
No background runner is installed; intervals are active-session targets.
Client options are forwarded exactly, without changing permissions or login.
HELP
}

fail() { printf 'workflow: %s\n' "$*" >&2; exit 2; }
if [[ "${1:-}" == --help || "${1:-}" == -h ]]; then usage; exit 0; fi
[[ $# -gt 0 ]] || { usage >&2; exit 2; }
client="$1"
shift
case "$client" in codex|claude) ;; *) fail "client must be codex or claude" ;; esac
status_interval=300
triage_interval=900
max_agents=2
max_heavy_tests=1
while [[ $# -gt 0 ]]; do
  case "$1" in
    --) shift; break ;;
    --status-interval|--triage-interval|--max-agents|--max-heavy-tests)
      [[ $# -ge 2 && "$2" =~ ^[1-9][0-9]*$ ]] || fail "$1 requires a positive integer"
      case "$1" in
        --status-interval) status_interval="$2" ;;
        --triage-interval) triage_interval="$2" ;;
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
prompt="Use the Taxiway contribution workflow. Read .agents/skills/taxiway-workflow/SKILL.md and AGENTS.md before acting. Start the intake dialogue: ask whether I have a new need or problem to analyze, or want suggestions from the repository backlog. Reconcile existing work before assigning anything. Target overall status updates every $status_interval seconds and backlog triage every $triage_interval seconds while this session is active, including unchanged-but-running work. Limit concurrency to $max_agents agents including the coordinator and $max_heavy_tests heavy Docker/Lima test runs, further reduced by actual available capacity. Tell me the next expected update and any client limitation preventing these targets. Do not start a subject or create issues before I authorize that work; publishing and merging need explicit authorization. Use one canonical workflow for both clients."
exec "$client" "$@" "$prompt"
