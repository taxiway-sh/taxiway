#!/usr/bin/env bash
# Commit and push a scheduled agent's heartbeat, status/<id>.json, to main of
# the taxiway-sh/agents state repository: set its date to now, validate the
# whole state, commit only that file and push without force, rebasing on a
# failed push. Prints the pushed commit SHA.
# Usage: scripts/agents/push-heartbeat.sh <id> [agents-clone-dir]
set -euo pipefail

usage() {
  cat <<'HELP'
Usage: scripts/agents/push-heartbeat.sh <id> [agents-clone-dir]

Run from the taxiway clone after writing status/<id>.json in the agents clone
(default: ../agents next to the taxiway clone). Fast-forwards the clone to
origin/main, sets the heartbeat's date to the current UTC time, validates the
state with scripts/check-status.py from origin/main, commits only
status/<id>.json and pushes it to origin main without force. On failure after
the commit, it undoes its own commit and leaves the heartbeat staged.
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
# Every git command on the clone goes through g: the clone's hooks and file
# system monitor would otherwise run with whatever approval this script got.
g() { git -C "$clone" -c core.hooksPath=/dev/null -c core.fsmonitor=false "$@"; }
top="$(g rev-parse --show-toplevel 2>/dev/null)" || fail "$clone_arg is not a git repository"
[[ "$(cd "$top" && pwd -P)" == "$clone" ]] || fail "$clone_arg is not the root of the agents clone"
# The clone's own git configuration may hold only what a plain clone writes:
# other keys (includes, filters, signing or ssh programs, URL rewrites) could
# run code or redirect the push.
unexpected="$(g config --local --name-only --list | grep -vE \
  '^(core\.(repositoryformatversion|filemode|bare|logallrefupdates|ignorecase|precomposeunicode|symlinks)|remote\.origin\.(url|fetch)|branch\..+\.(remote|merge)|gc\.auto|user\.(name|email))$' \
  | sort -u || true)"
[[ -z "$unexpected" ]] || fail "the agents clone's git config sets keys this script does not allow:
$unexpected"
origin="$(g config --local --get-all remote.origin.url)" || fail "the agents clone has no origin remote"
[[ "$origin" != *$'\n'* ]] || fail "remote.origin.url is set more than once in the agents clone"
case "$origin" in
  https://github.com/taxiway-sh/agents|https://github.com/taxiway-sh/agents.git) ;;
  git@github.com:taxiway-sh/agents.git|ssh://git@github.com/taxiway-sh/agents.git) ;;
  *) fail "the clone's origin is $origin, not taxiway-sh/agents" ;;
esac
branch="$(g symbolic-ref --quiet --short HEAD)" || fail "the agents clone is not on a branch; check out main"
[[ "$branch" == main ]] || fail "the agents clone is on '$branch', not main"
git_dir="$(g rev-parse --absolute-git-dir)"
[[ ! -e "$git_dir/rebase-merge" && ! -e "$git_dir/rebase-apply" ]] || fail "a rebase is in progress in the agents clone"

heartbeat="status/$id.json"
[[ -f "$clone/$heartbeat" ]] || fail "$heartbeat not found in the agents clone: write it first"
# Only the heartbeat may differ from HEAD: anything else staged or modified
# would be committed with it, or would block the rebase on a rejected push.
others="$( (g diff --name-only; g diff --cached --name-only) \
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

# Fetch and push by the checked URL with explicit refspecs, whatever the
# clone's remote settings say.
fetch() { g fetch --quiet "$origin" +refs/heads/main:refs/remotes/origin/main; }
fetch || fail "git fetch origin main failed; nothing was committed"
# Anything already committed locally would be pushed with the heartbeat.
[[ "$(g rev-list --count origin/main..HEAD)" == 0 ]] \
  || fail "the agents clone has commits that are not on origin/main; push or drop them first"
g merge --quiet --ff-only origin/main \
  || fail "cannot fast-forward the agents clone to origin/main; nothing was committed"
python3 -I -c 'import jsonschema' 2>/dev/null \
  || fail "python3 -I cannot import jsonschema, needed to validate the state; install it for the system python3 (user site-packages are ignored)"
# Run the validator from origin/main, never from the working tree, so local
# edits to it cannot run under this script.
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
g show origin/main:scripts/check-status.py > "$tmp/check-status.py" 2>/dev/null \
  || fail "scripts/check-status.py not found on origin/main of the agents repository"
python3 -I "$tmp/check-status.py" "$clone" \
  "$agents_dir/status.schema.json" "$agents_dir/queue.schema.json" \
  || fail "state validation failed (see above); nothing was committed"

g add -- "$heartbeat"
if g diff --cached --quiet; then
  fail "$heartbeat is unchanged since the last commit; nothing to commit"
fi
g commit --quiet -m "chore($id): record run heartbeat" || fail "git commit failed"

# On failure, undo this script's own unpushed commit, keeping the heartbeat
# staged, so the clone can still fast-forward at the next run.
abandon() {
  g reset --quiet --soft HEAD~1 || true
  fail "$1; the heartbeat is not pushed and its commit was undone"
}
retries=0
until g push --quiet "$origin" HEAD:refs/heads/main; do
  (( retries < max_retries )) || abandon "push failed $((retries + 1)) times"
  retries=$((retries + 1))
  printf 'push-heartbeat: push failed, rebasing on origin/main (retry %d of %d)\n' "$retries" "$max_retries" >&2
  fetch || abandon "git fetch origin main failed"
  if ! g rebase --quiet origin/main; then
    g rebase --abort || true
    abandon "rebase on origin/main failed"
  fi
done
g rev-parse HEAD
