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

heartbeat() { # id result
  cat > "$agents/status/$1.json" <<JSON
{"id": "$1", "date": "2000-01-01T00:00:00Z", "result": "$2", "summary": "Test run.",
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

# Success, with the default clone directory, run from the taxiway clone.
heartbeat demo ok
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
assert data["summary"] == "Test run." and data["frictions"] == [], data
PY
echo 'PASS: date set to now, only the heartbeat committed and pushed, SHA printed'

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

# Push race: another agent pushed its heartbeat since this clone last fetched.
other="$test_dir/other"
git clone --quiet https://github.com/taxiway-sh/agents "$other"
cp "$agents/status/demo.json" "$other/status/other.json"
sed -i 's/"demo"/"other"/' "$other/status/other.json"
git -C "$other" add status/other.json
git -C "$other" commit --quiet -m 'chore(other): record run heartbeat'
git -C "$other" push --quiet origin main
theirs="$(origin_head)"
heartbeat demo partial
sha="$("$script" demo "$agents" 2>"$test_dir/race.err")"
grep -q 'push rejected, rebasing' "$test_dir/race.err" || { echo 'FAIL: no rejected push in the race' >&2; exit 1; }
[[ "$(origin_head)" == "$sha" && "$(git --git-dir="$origin" rev-parse "$sha^")" == "$theirs" ]] \
  || { echo 'FAIL: heartbeat not rebased onto the other push' >&2; exit 1; }
echo 'PASS: rejected push rebased and retried'

# A push that keeps failing stops after three retries, without force.
cat > "$origin/hooks/pre-receive" <<'SH'
#!/usr/bin/env bash
echo rejected >&2
exit 1
SH
chmod +x "$origin/hooks/pre-receive"
pushed="$(origin_head)"
heartbeat demo failed
expect_fail 'push rejected 4 times.*not pushed' "$script" demo "$agents"
[[ "$(origin_head)" == "$pushed" ]] || { echo 'FAIL: origin changed' >&2; exit 1; }
echo 'PASS: gives up after three retries'
