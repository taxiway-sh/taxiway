#!/usr/bin/env bash
set -euo pipefail
repo_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
test_dir="$(mktemp -d)"
trap 'rm -rf "$test_dir"' EXIT

# Fixture: a taxiway tree with the script and the real schemas, and the agents
# clone next to it, whose origin URL is taxiway-sh/agents but resolves to a
# local bare repository through the test's own git configuration.
taxiway="$test_dir/work/taxiway"
agents="$test_dir/work/agents"
origin="$test_dir/origin.git"
script="$taxiway/scripts/agents/push-heartbeat.sh"
mkdir -p "$taxiway/scripts/agents" "$taxiway/.agents/agents"
cp "$repo_dir/scripts/agents/push-heartbeat.sh" "$script"
cp "$repo_dir/.agents/agents/status.schema.json" "$repo_dir/.agents/agents/queue.schema.json" \
  "$taxiway/.agents/agents/"
echo '{}' > "$taxiway/.agents/agents/demo.json"
echo '{}' > "$taxiway/.agents/agents/other.json"

export GIT_CONFIG_NOSYSTEM=1 GIT_CONFIG_GLOBAL="$test_dir/gitconfig"
git config --global user.name 'Test Agent'
git config --global user.email 'agent@example.invalid'
git config --global init.defaultBranch main
git config --global "url.$origin.insteadOf" https://github.com/taxiway-sh/agents
git init --quiet --bare "$origin"
# The script requires jsonschema, which the real validator imports, and runs
# Python isolated (-I). The stand-in below does not need jsonschema: this
# python3 wrapper provides an empty module where it is not installed.
real_python="$(command -v python3)"
mkdir -p "$test_dir/py" "$test_dir/bin"
touch "$test_dir/py/jsonschema.py"
cat > "$test_dir/bin/python3" <<SH
#!/usr/bin/env bash
args=()
for arg in "\$@"; do [[ "\$arg" == -I ]] || args+=("\$arg"); done
PYTHONPATH="$test_dir/py" exec "$real_python" "\${args[@]}"
SH
chmod +x "$test_dir/bin/python3"
export PATH="$test_dir/bin:$PATH"

# The state repository's validator is not available here: this stand-in checks
# the arguments it receives and the heartbeat fields this test relies on.
seed="$test_dir/seed"
git init --quiet "$seed"
mkdir -p "$seed/scripts" "$seed/status"
cat > "$seed/scripts/check-status.py" <<'PY'
import json, pathlib, sys
root, status_schema, queue_schema = map(pathlib.Path, sys.argv[1:])
assert status_schema.name == "status.schema.json" and status_schema.is_file()
assert queue_schema.name == "queue.schema.json" and queue_schema.is_file()
schema = json.loads(status_schema.read_text())
failed = False
for path in sorted((root / "status").glob("*.json")):
    data = json.loads(path.read_text())
    missing = set(schema["required"]) - data.keys()
    if missing or data["id"] != path.stem or data["result"] not in schema["properties"]["result"]["enum"]:
        print(f"FAIL: status/{path.name}: invalid heartbeat", file=sys.stderr)
        failed = True
sys.exit(1 if failed else 0)
PY
echo '{"agents": {}}' > "$seed/deployed.json"
touch "$seed/status/.gitkeep"
git -C "$seed" add -A
git -C "$seed" commit --quiet -m 'chore: seed state'
git -C "$seed" push --quiet https://github.com/taxiway-sh/agents main
git clone --quiet https://github.com/taxiway-sh/agents "$agents"

runs=0
heartbeat() { # id result; each heartbeat differs, even within one second
  runs=$((runs + 1))
  cat > "$agents/status/$1.json" <<JSON
{"id": "$1", "date": "2000-01-01T00:00:00Z", "result": "$2", "summary": "Test run $runs.",
 "waiting": [], "promptVersion": "$(printf '0%.0s' {1..40})", "frictions": []}
JSON
}
expect_fail() { # message-pattern command...
  local pattern="$1" out
  shift
  if out="$("$@" 2>&1)"; then
    echo "FAIL: accepted: $*" >&2
    exit 1
  fi
  if ! grep -qE "$pattern" <<<"$out"; then
    echo "FAIL: expected /$pattern/ from $*, got: $out" >&2
    exit 1
  fi
}
origin_head() { git --git-dir="$origin" rev-parse main; }

# Success, with the default clone directory, run from the taxiway clone. An
# untracked file is left alone, and the clone's hooks do not run.
heartbeat demo ok
echo scratch > "$agents/notes.txt"
printf '#!/usr/bin/env bash\ntouch "%s/hook-ran"\n' "$test_dir" > "$agents/.git/hooks/pre-commit"
cp "$agents/.git/hooks/pre-commit" "$agents/.git/hooks/pre-push"
chmod +x "$agents/.git/hooks/pre-commit" "$agents/.git/hooks/pre-push"
before="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
sha="$(cd "$taxiway" && scripts/agents/push-heartbeat.sh demo)"
after="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
[[ "$sha" =~ ^[0-9a-f]{40}$ ]] || { echo "FAIL: printed '$sha', not a commit SHA" >&2; exit 1; }
[[ "$(origin_head)" == "$sha" ]] || { echo 'FAIL: origin main is not the printed SHA' >&2; exit 1; }
[[ "$(git --git-dir="$origin" log -1 --format=%B main)" == 'chore(demo): record run heartbeat' ]] \
  || { echo 'FAIL: commit message is not exactly the subject, without trailers' >&2; exit 1; }
[[ "$(git --git-dir="$origin" diff-tree --no-commit-id --name-only -r main)" == status/demo.json ]] \
  || { echo 'FAIL: commit must change only status/demo.json' >&2; exit 1; }
python3 - "$agents/status/demo.json" "$before" "$after" <<'PY'
import json, sys
data = json.load(open(sys.argv[1]))
assert sys.argv[2] <= data["date"] <= sys.argv[3], data["date"]
assert data["summary"] == "Test run 1." and data["frictions"] == [], data
PY
[[ -f "$agents/notes.txt" && ! -e "$test_dir/hook-ran" ]] \
  || { echo 'FAIL: untracked file touched or a clone hook ran' >&2; exit 1; }
rm "$agents/notes.txt" "$agents/.git/hooks/pre-commit" "$agents/.git/hooks/pre-push"
echo 'PASS: date set to now, only the heartbeat committed and pushed, SHA printed, no hooks'

# Bad ids.
for id in 'Demo' '9demo' '../demo' 'demo/x' '-demo' ''; do
  expect_fail 'invalid agent id' "$script" "$id" "$agents"
done
expect_fail 'no declaration' "$script" missing "$agents"
expect_fail 'Usage' "$script"
echo 'PASS: malformed and undeclared ids rejected'

# Wrong clone: not the agents origin, not on main.
git clone --quiet "$origin" "$test_dir/foreign"
expect_fail 'not taxiway-sh/agents' "$script" demo "$test_dir/foreign"
git -C "$agents" switch --quiet -c side
expect_fail "not main" "$script" demo "$agents"
git -C "$agents" switch --quiet main
echo 'PASS: foreign origin and other branch rejected'

# Local git config beyond what a clone writes is refused before anything runs.
printf '#!/usr/bin/env bash\ntouch "%s/fsmonitor-ran"\nexit 1\n' "$test_dir" > "$test_dir/monitor"
chmod +x "$test_dir/monitor"
git -C "$agents" config core.fsmonitor "$test_dir/monitor"
expect_fail '^core\.fsmonitor$' "$script" demo "$agents"
git -C "$agents" config --unset core.fsmonitor
git -C "$agents" config include.path "$test_dir/gitconfig"
expect_fail '^include\.path$' "$script" demo "$agents"
git -C "$agents" config --unset include.path
[[ ! -e "$test_dir/fsmonitor-ran" ]] || { echo 'FAIL: the clone fsmonitor ran' >&2; exit 1; }
git -C "$agents" config --add remote.origin.url https://github.com/taxiway-sh/agents
expect_fail 'set more than once' "$script" demo "$agents"
git -C "$agents" config --unset-all remote.origin.url
git -C "$agents" config remote.origin.url https://github.com/taxiway-sh/agents
echo 'PASS: unexpected local git config refused'

# Invalid heartbeat: validation fails, nothing is committed or pushed.
pushed="$(origin_head)"
heartbeat demo great
expect_fail 'state validation failed' "$script" demo "$agents"
echo 'not json' > "$agents/status/demo.json"
expect_fail 'cannot set the date' "$script" demo "$agents"
[[ "$(git -C "$agents" rev-parse HEAD)" == "$pushed" && "$(origin_head)" == "$pushed" ]] \
  || { echo 'FAIL: an invalid heartbeat was committed or pushed' >&2; exit 1; }
git -C "$agents" checkout --quiet -- status/demo.json
echo 'PASS: invalid heartbeat rejected before commit'

# Another modified or staged file: refused, nothing committed.
heartbeat demo ok
echo '{"agents": {"x": {}}}' > "$agents/deployed.json"
expect_fail 'other changes' "$script" demo "$agents"
git -C "$agents" add deployed.json
expect_fail 'other changes' "$script" demo "$agents"
[[ "$(git -C "$agents" rev-parse HEAD)" == "$pushed" ]] || { echo 'FAIL: committed with another change' >&2; exit 1; }
git -C "$agents" reset --quiet -- deployed.json
git -C "$agents" checkout --quiet -- deployed.json
echo 'PASS: other staged or modified files refused'

# A local commit that is not on origin/main would be pushed with the heartbeat:
# refused.
git -C "$agents" stash --quiet
echo '{"agents": {}}' > "$agents/deployed.json"
git -C "$agents" commit --quiet --allow-empty -m 'local change'
expect_fail 'not on origin/main' "$script" demo "$agents"
git -C "$agents" reset --quiet --hard origin/main
git -C "$agents" stash pop --quiet
echo 'PASS: unpushed local commits refused'

# The validator runs from origin/main, not from the clone's working tree.
updater="$test_dir/updater"
git clone --quiet https://github.com/taxiway-sh/agents "$updater"
sed -i "1i open('$test_dir/origin-validator-ran', 'w')" "$updater/scripts/check-status.py"
git -C "$updater" commit --quiet -am 'validator update'
git -C "$updater" push --quiet origin main
"$script" demo "$agents" >/dev/null 2>"$test_dir/ff.err" || { cat "$test_dir/ff.err" >&2; exit 1; }
[[ -e "$test_dir/origin-validator-ran" ]] || { echo 'FAIL: the validator did not come from origin/main' >&2; exit 1; }
! grep -q 'push failed' "$test_dir/ff.err" \
  || { echo 'FAIL: the clone was not fast-forwarded before the commit' >&2; exit 1; }
# A local edit of the validator hidden from git diff does not run either.
git -C "$agents" update-index --skip-worktree scripts/check-status.py
sed -i "1i open('$test_dir/local-validator-ran', 'w')" "$agents/scripts/check-status.py"
heartbeat demo ok
"$script" demo "$agents" >/dev/null
[[ ! -e "$test_dir/local-validator-ran" ]] || { echo 'FAIL: the local validator ran' >&2; exit 1; }
git -C "$agents" update-index --no-skip-worktree scripts/check-status.py
git -C "$agents" checkout --quiet -- scripts/check-status.py
# A fetch refspec that would leave origin/main stale does not change what is
# fetched: the next validator update on origin still runs.
git -C "$agents" config remote.origin.fetch +refs/heads/none:refs/remotes/origin/none
git -C "$updater" pull --quiet --ff-only origin main
sed -i "1i open('$test_dir/origin-validator-2-ran', 'w')" "$updater/scripts/check-status.py"
git -C "$updater" commit --quiet -am 'validator update 2'
git -C "$updater" push --quiet origin main
heartbeat demo ok
"$script" demo "$agents" >/dev/null
[[ -e "$test_dir/origin-validator-2-ran" && "$(origin_head)" == "$(git -C "$agents" rev-parse HEAD)" ]] \
  || { echo 'FAIL: a custom fetch refspec changed what was validated or pushed' >&2; exit 1; }
git -C "$agents" config remote.origin.fetch '+refs/heads/*:refs/remotes/origin/*'
echo 'PASS: clone fast-forwarded, validator taken from origin/main'

# Without jsonschema, the script says so instead of reporting a bad state.
mkdir -p "$test_dir/nojs"
printf '#!/usr/bin/env bash\n[[ "$*" == "-I -c import jsonschema" ]] && exit 1\nexec %q "$@"\n' \
  "$test_dir/bin/python3" > "$test_dir/nojs/python3"
chmod +x "$test_dir/nojs/python3"
heartbeat demo ok
PATH="$test_dir/nojs:$PATH" expect_fail 'cannot import jsonschema' "$script" demo "$agents"
echo 'PASS: missing jsonschema reported'

# Push race: another agent's heartbeat lands on main between this script's
# fetch and its push. A one-shot pre-receive hook moves main to that commit,
# prepared on a side branch, then rejects the push.
other="$test_dir/other"
git clone --quiet https://github.com/taxiway-sh/agents "$other"
cp "$agents/status/demo.json" "$other/status/other.json"
sed -i 's/"demo"/"other"/' "$other/status/other.json"
git -C "$other" add status/other.json
git -C "$other" commit --quiet -m 'chore(other): record run heartbeat'
git -C "$other" push --quiet origin HEAD:refs/heads/side
theirs="$(git -C "$other" rev-parse HEAD)"
race_hook() { # commit that lands on main during the next push
  rm -f "$test_dir/raced"
  cat > "$origin/hooks/pre-receive" <<SH
#!/usr/bin/env bash
[[ -e "$test_dir/raced" ]] && exit 0
touch "$test_dir/raced"
env -u GIT_QUARANTINE_PATH -u GIT_OBJECT_DIRECTORY -u GIT_ALTERNATE_OBJECT_DIRECTORIES \
  git --git-dir="$origin" update-ref refs/heads/main "$1"
exit 1
SH
  chmod +x "$origin/hooks/pre-receive"
}
race_hook "$theirs"
heartbeat demo partial
sha="$("$script" demo "$agents" 2>"$test_dir/race.err")"
grep -q 'push failed, rebasing' "$test_dir/race.err" || { echo 'FAIL: no failed push in the race' >&2; exit 1; }
[[ "$(origin_head)" == "$sha" && "$(git --git-dir="$origin" rev-parse "$sha^")" == "$theirs" ]] \
  || { echo 'FAIL: heartbeat not rebased onto the other push' >&2; exit 1; }
echo 'PASS: rejected push rebased and retried'

# A rebase conflict stops, and the script undoes its own commit.
git -C "$other" fetch --quiet origin main
git -C "$other" reset --quiet --hard origin/main
python3 - "$other/status/demo.json" <<'PY'
import json, sys
data = json.load(open(sys.argv[1]))
data["summary"] = "Edited elsewhere."
open(sys.argv[1], "w").write(json.dumps(data) + "\n")
PY
git -C "$other" commit --quiet -am 'edit demo elsewhere'
git -C "$other" push --quiet origin HEAD:refs/heads/side2
race_hook "$(git -C "$other" rev-parse HEAD)"
before="$(git -C "$agents" rev-parse HEAD)"
heartbeat demo ok
expect_fail 'rebase on origin/main failed.*commit was undone' "$script" demo "$agents"
[[ "$(git -C "$agents" rev-parse HEAD)" == "$before" && "$(git -C "$agents" diff --cached --name-only)" == status/demo.json ]] \
  || { echo 'FAIL: the failed heartbeat commit was not undone' >&2; exit 1; }
echo 'PASS: rebase conflict undoes the commit'

# A push that keeps failing stops after three retries, without force, and
# undoes the commit.
git -C "$agents" reset --quiet --hard origin/main
cat > "$origin/hooks/pre-receive" <<'SH'
#!/usr/bin/env bash
echo rejected >&2
exit 1
SH
pushed="$(origin_head)"
heartbeat demo failed
expect_fail 'push failed 4 times.*commit was undone' "$script" demo "$agents"
[[ "$(origin_head)" == "$pushed" && "$(git -C "$agents" rev-parse HEAD)" == "$pushed" ]] \
  || { echo 'FAIL: origin changed or the commit was not undone' >&2; exit 1; }
echo 'PASS: gives up after three retries'
