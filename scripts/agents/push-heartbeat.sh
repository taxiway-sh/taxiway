#!/usr/bin/env bash
# Commit and push a scheduled agent's heartbeat, status/<id>.json, to main of
# the taxiway-sh/agents state repository: set its date to now, validate the
# whole state, commit only that file and push without force, rebasing on a
# rejected push. Prints the pushed commit SHA.
# Usage: scripts/agents/push-heartbeat.sh <id> [agents-clone-dir]
set -euo pipefail

usage() {
  cat <<'HELP'
Usage: scripts/agents/push-heartbeat.sh <id> [agents-clone-dir]

Run from the taxiway clone after writing status/<id>.json in the agents clone
(default: ../agents next to the taxiway clone). Sets the heartbeat's date to
the current UTC time, validates the state with the agents repository's
scripts/check-status.py, commits only status/<id>.json and pushes it to
origin main without force.
HELP
}

fail() { printf 'push-heartbeat: %s\n' "$*" >&2; exit 1; }
max_retries=3

if [[ "${1:-}" == --help || "${1:-}" == -h ]]; then usage; exit 0; fi
[[ $# -ge 1 && $# -le 2 ]] || { usage >&2; exit 2; }
id="$1"
[[ "$id" =~ ^[a-z][a-z0-9-]*$ ]] || fail "invalid agent id '$id': expected ^[a-z][a-z0-9-]*\$"
repo_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)"
agents_dir="$repo_dir/.agents/agents"
[[ -f "$agents_dir/$id.json" ]] || fail "no declaration .agents/agents/$id.json for agent '$id'"
for schema in status.schema.json queue.schema.json; do
  [[ -f "$agents_dir/$schema" ]] || fail "missing .agents/agents/$schema"
done

clone_arg="${2:-$repo_dir/../agents}"
[[ -d "$clone_arg" ]] || fail "agents clone not found: $clone_arg"
clone="$(cd "$clone_arg" && pwd -P)"
top="$(git -C "$clone" rev-parse --show-toplevel 2>/dev/null)" || fail "$clone_arg is not a git repository"
[[ "$(cd "$top" && pwd -P)" == "$clone" ]] || fail "$clone_arg is not the root of the agents clone"
origin="$(git -C "$clone" config --get remote.origin.url)" || fail "the agents clone has no origin remote"
case "$origin" in
  https://github.com/taxiway-sh/agents|https://github.com/taxiway-sh/agents.git) ;;
  git@github.com:taxiway-sh/agents.git|ssh://git@github.com/taxiway-sh/agents.git) ;;
  *) fail "the clone's origin is $origin, not taxiway-sh/agents" ;;
esac
branch="$(git -C "$clone" symbolic-ref --quiet --short HEAD)" || fail "the agents clone is not on a branch; check out main"
[[ "$branch" == main ]] || fail "the agents clone is on '$branch', not main"
git_dir="$(git -C "$clone" rev-parse --absolute-git-dir)"
[[ ! -e "$git_dir/rebase-merge" && ! -e "$git_dir/rebase-apply" ]] || fail "a rebase is in progress in the agents clone"

heartbeat="status/$id.json"
[[ -f "$clone/$heartbeat" ]] || fail "$heartbeat not found in the agents clone: write it first"
# Only the heartbeat may differ from HEAD: anything else staged or modified
# would be committed with it, or would block the rebase on a rejected push.
others="$( (git -C "$clone" diff --name-only; git -C "$clone" diff --cached --name-only) \
  | grep -vxF "$heartbeat" | sort -u || true)"
[[ -z "$others" ]] || fail "other changes in the agents clone, commit or revert them first:
$others"

now="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
python3 -I - "$clone/$heartbeat" "$now" <<'PY' || fail "cannot set the date of $heartbeat (see above)"
import json
import pathlib
import sys

path = pathlib.Path(sys.argv[1])
try:
    data = json.loads(path.read_text())
except ValueError as err:
    sys.exit(f"invalid JSON: {err}")
if not isinstance(data, dict):
    sys.exit("not a JSON object")
data["date"] = sys.argv[2]
path.write_text(json.dumps(data, indent=2, ensure_ascii=False) + "\n")
PY

[[ -f "$clone/scripts/check-status.py" ]] || fail "scripts/check-status.py not found in the agents clone"
python3 "$clone/scripts/check-status.py" "$clone" \
  "$agents_dir/status.schema.json" "$agents_dir/queue.schema.json" \
  || fail "state validation failed (see above); nothing was committed"

git -C "$clone" add -- "$heartbeat"
if git -C "$clone" diff --cached --quiet; then
  fail "$heartbeat is unchanged; nothing to commit"
fi
git -C "$clone" commit --quiet -m "chore($id): record run heartbeat" || fail "git commit failed"

retries=0
until git -C "$clone" push --quiet origin main; do
  (( retries < max_retries )) || fail "push rejected $((retries + 1)) times; the heartbeat commit $(git -C "$clone" rev-parse HEAD) is not pushed"
  retries=$((retries + 1))
  printf 'push-heartbeat: push rejected, rebasing on origin/main (retry %d of %d)\n' "$retries" "$max_retries" >&2
  git -C "$clone" fetch --quiet origin main || fail "git fetch origin main failed; the heartbeat is not pushed"
  if ! git -C "$clone" rebase --quiet origin/main; then
    git -C "$clone" rebase --abort || true
    fail "rebase on origin/main failed; the heartbeat is not pushed"
  fi
done
git -C "$clone" rev-parse HEAD
