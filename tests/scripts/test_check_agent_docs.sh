#!/usr/bin/env bash
set -euo pipefail
repo_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
checker="$repo_dir/scripts/check-agent-docs.sh"
test_dir="$(mktemp -d)"
trap 'rm -rf "$test_dir"' EXIT

fixture() {
  local tree="$test_dir/$1"
  mkdir -p "$tree/.agents/skills/demo" "$tree/.agents/skills/taxiway-testing" "$tree/.claude/skills" \
    "$tree/docs/contributing" "$tree/tests/live"
  cat > "$tree/AGENTS.md" <<'MD'
# Agents

Each rule has one owner; link to it instead of restating it.

Follow the [E2E scenario rule](.agents/skills/taxiway-testing/SKILL.md#e2e-scenarios).
See [testing](docs/contributing/testing.md#writing-tests), the
[skill](.agents/skills/demo/SKILL.md), [helpers](docs/contributing/testing.md#run-test_helperssh),
[emphasis](docs/contributing/testing.md#emphasis-word),
[code](docs/contributing/testing.md#failed-to-mount-overlay),
[mixed](docs/contributing/testing.md#run-__init__--co-docs) and
[custom](docs/contributing/testing.md#custom-anchor).
MD
  cat > "$tree/docs/contributing/testing.md" <<'MD'
# Testing

## Writing tests

Follow the [E2E scenario rule](https://github.com/taxiway-sh/taxiway/blob/main/.agents/skills/taxiway-testing/SKILL.md#e2e-scenarios).
Other sites are not checked: [example](https://example.invalid/missing.md).
```text
[not a link](missing.md)
```
````markdown
```
[still not a link](missing.md)
````

## Run test_helpers.sh

## _Emphasis_ word

### `failed to mount overlay`

## Run `__init__` &amp; <b>co</b> [docs](testing.md)

<a id="custom-anchor"></a>
MD
  cat > "$tree/.agents/skills/demo/SKILL.md" <<'MD'
---
name: demo
description: >-
  Use for the fixture.
---

Read [the rule](../taxiway-testing/SKILL.md#e2e-scenarios).
MD
  cat > "$tree/.agents/skills/taxiway-testing/SKILL.md" <<'MD'
---
name: taxiway-testing
description: Use for the fixture rule owner.
---

## E2E scenarios

Each orchestrator has three lifecycle E2E scenarios. Adding an orchestrator
adds its own three scenarios. Do not add parallel suites or `TestE2E_` entry points.
MD
  printf '# Live\n' > "$tree/tests/live/AGENTS.md"
  ln -s ../../.agents/skills/demo "$tree/.claude/skills/demo"
  ln -s ../../.agents/skills/taxiway-testing "$tree/.claude/skills/taxiway-testing"
  mkdir -p "$tree/.agents/skills/taxiway-agents"
  cat > "$tree/.agents/skills/taxiway-agents/SKILL.md" <<'MD'
---
name: taxiway-agents
description: Use for the fixture.
---

### Action classes

| Class | Covers | Highest level |
|---|---|---|
| `comment-issue` | Commenting | `auto` |
| `push-pr-branch` | Pushing a PR branch | `auto` |
| `merge-pr` | Merging | `ask` |
| `change-agent-system` | Changing the agentic system | `ask` |

### Never

| Action | Never |
|---|---|
| `push-main` | Pushing to main |
| `force-push-foreign` | Force-pushing |
| `publish-release` | Releases |
| `publish-upstream` | Upstream |
| `handle-secrets` | Secrets |
| `skip-tests` | Skipping tests |
| `change-own-instructions` | Own instructions |
| `change-approvals` | Approvals |
| `change-never-list` | This list |

## After
MD
  ln -s ../../.agents/skills/taxiway-agents "$tree/.claude/skills/taxiway-agents"
  mkdir -p "$tree/.agents/agents"
  cat > "$tree/.agents/agents/watch.json" <<'JSON'
{"id": "watch", "title": "Taxiway – watch agent (fixture)", "schedule": "0 4 * * *", "model": "default",
 "repositories": {"taxiway-sh/taxiway": "read", "taxiway-sh/agents": "write"},
 "workdir": "taxiway", "skills": ["demo"], "outputs": ["report"],
 "autonomy": {"comment-issue": "auto", "merge-pr": "ask"}}
JSON
  printf '# Watch\n' > "$tree/.agents/agents/watch.prompt.md"
  cp "$repo_dir/.agents/agents/status.schema.json" "$tree/.agents/agents/status.schema.json"
  cp "$repo_dir/.agents/agents/queue.schema.json" "$tree/.agents/agents/queue.schema.json"
  printf '%s\n' "$tree"
}

expect_fail() {
  local name="$1" tree="$2" message="$3"
  if bash "$checker" "$tree" > "$test_dir/out" 2>&1; then
    echo "FAIL: $name was accepted" >&2
    exit 1
  fi
  if ! grep -qF -- "$message" "$test_dir/out"; then
    echo "FAIL: $name did not report: $message" >&2
    cat "$test_dir/out" >&2
    exit 1
  fi
  echo "PASS: $name rejected"
}

tree="$(fixture pass)"
bash "$checker" "$tree" > "$test_dir/out"
grep -q 'check-agent-docs: OK' "$test_dir/out"
echo 'PASS: valid tree accepted'

tree="$(fixture dead-link)"
printf 'See [gone](gone.md).\n' >> "$tree/docs/contributing/testing.md"
expect_fail 'dead relative link' "$tree" 'docs/contributing/testing.md: dead link gone.md'

tree="$(fixture dead-anchor)"
printf 'See [nowhere](../../../AGENTS.md#nowhere).\n' >> "$tree/.agents/skills/demo/SKILL.md"
expect_fail 'missing anchor' "$tree" '.agents/skills/demo/SKILL.md: missing anchor ../../../AGENTS.md#nowhere'

tree="$(fixture blob-anchor)"
printf 'See [rule](https://github.com/taxiway-sh/taxiway/blob/main/AGENTS.md#nowhere).\n' >> "$tree/docs/contributing/testing.md"
expect_fail 'GitHub blob link to a missing anchor' "$tree" 'missing anchor https://github.com/taxiway-sh/taxiway/blob/main/AGENTS.md#nowhere'

tree="$(fixture docs-relative-outside)"
printf 'See [agents](../../AGENTS.md).\n' >> "$tree/docs/contributing/testing.md"
expect_fail 'relative link from docs/ to a file outside docs/' "$tree" 'docs/contributing/testing.md: relative link ../../AGENTS.md leaves docs/'

tree="$(fixture duplicate)"
printf 'Do not add parallel\nsuites here either.\n' >> "$tree/tests/live/AGENTS.md"
expect_fail 'duplicated normative phrase' "$tree" "tests/live/AGENTS.md: restates 'parallel suite'"

tree="$(fixture e2e-rule-in-agents)"
printf 'Adding an orchestrator adds its own\nthree scenarios.\n' >> "$tree/AGENTS.md"
expect_fail 'E2E rule restated outside its skill' "$tree" "AGENTS.md: restates 'adds its own three scenarios'; link to .agents/skills/taxiway-testing/SKILL.md"

tree="$(fixture no-frontmatter)"
printf '# Demo\n\nNo frontmatter.\n' > "$tree/.agents/skills/demo/SKILL.md"
expect_fail 'skill without frontmatter' "$tree" '.agents/skills/demo/SKILL.md: missing YAML frontmatter'

tree="$(fixture empty-block-description)"
printf -- '---\nname: demo\ndescription: >\n---\n' > "$tree/.agents/skills/demo/SKILL.md"
expect_fail 'empty folded description' "$tree" 'frontmatter description is empty'

tree="$(fixture wrong-name)"
sed -i.bak 's/^name: demo$/name: other/' "$tree/.agents/skills/demo/SKILL.md"
rm "$tree/.agents/skills/demo/SKILL.md.bak"
expect_fail 'skill name mismatch' "$tree" "frontmatter name must be 'demo'"

tree="$(fixture no-symlink)"
rm "$tree/.claude/skills/demo"
expect_fail 'missing discovery symlink' "$tree" '.claude/skills/demo: must be a symlink'

tree="$(fixture orphan-symlink)"
ln -s ../../.agents/skills/gone "$tree/.claude/skills/gone"
expect_fail 'orphan discovery entry' "$tree" '.claude/skills/gone: no matching .agents/skills/gone'

tree="$(fixture agent-write-taxiway)"
sed -i.bak 's/"taxiway-sh\/taxiway": "read"/"taxiway-sh\/taxiway": "write"/' "$tree/.agents/agents/watch.json"
expect_fail 'write on taxiway without a pushing class' "$tree" "write on taxiway-sh/taxiway requires one of ['change-agent-system', 'push-pr-branch'] at ask or auto"

tree="$(fixture agent-no-prompt)"
rm "$tree/.agents/agents/watch.prompt.md"
expect_fail 'agent without prompt' "$tree" '.agents/agents/watch.json: missing watch.prompt.md'

tree="$(fixture agent-unknown-skill)"
sed -i.bak 's/"demo"\]/"gone"]/' "$tree/.agents/agents/watch.json"
expect_fail 'agent with unknown skill' "$tree" "unknown skill 'gone'"

tree="$(fixture agent-duplicate-schedule)"
sed 's/"watch"/"other"/; s/watch agent/other agent/' "$tree/.agents/agents/watch.json" > "$tree/.agents/agents/other.json"
printf '# Other\n' > "$tree/.agents/agents/other.prompt.md"
expect_fail 'two agents on one schedule' "$tree" "schedule '0 4 * * *' already used by"

tree="$(fixture agent-duplicate-title)"
sed 's/"watch"/"other"/; s/"0 4 \* \* \*"/"0 5 * * *"/' "$tree/.agents/agents/watch.json" > "$tree/.agents/agents/other.json"
printf '# Other\n' > "$tree/.agents/agents/other.prompt.md"
expect_fail 'two agents with one title' "$tree" "title 'Taxiway – watch agent (fixture)' already used by"

tree="$(fixture agent-title-hyphen)"
sed -i.bak 's/Taxiway – watch agent/Taxiway - watch agent/' "$tree/.agents/agents/watch.json"
expect_fail 'agent title without the en dash' "$tree" "title must be 'Taxiway – <role> agent (<scope>)'"

tree="$(fixture agent-title-old-form)"
sed -i.bak 's/"title": "[^"]*"/"title": "Taxiway agent: watch"/' "$tree/.agents/agents/watch.json"
expect_fail 'agent title in the old form' "$tree" "title must be 'Taxiway – <role> agent (<scope>)'"

for case in 'missing scope:Taxiway – watch agent ()' 'blank scope:Taxiway – watch agent ( )' \
    'double space:Taxiway – watch  agent (fixture)' 'padded scope:Taxiway – watch agent ( fixture)' \
    'watchdog title on another agent:Taxiway – agents watchdog'; do
  tree="$(fixture "agent-title-${case%%:*}")"
  TITLE="${case#*:}" python3 - "$tree/.agents/agents/watch.json" <<'PY2'
import json, os, sys
data = json.load(open(sys.argv[1])); data["title"] = os.environ["TITLE"]
json.dump(data, open(sys.argv[1], "w"))
PY2
  expect_fail "agent title with ${case%%:*}" "$tree" "title must be 'Taxiway – <role> agent (<scope>)'"
done

tree="$(fixture agent-title-format-character)"
python3 - "$tree/.agents/agents/watch.json" <<'PY2'
import json, sys
data = json.load(open(sys.argv[1])); data["title"] = "Taxiway \u2013 watch agent (fix\u200bture)"
json.dump(data, open(sys.argv[1], "w"))
PY2
expect_fail 'agent title with a zero-width character' "$tree" 'title must not contain invisible format characters'

for scope in 'fixture \u00e9t\u00e9' '\uff46ixture' 'fixture\u00a0x'; do
  tree="$(fixture agent-title-non-ascii-scope)"
  SCOPE="$scope" python3 - "$tree/.agents/agents/watch.json" <<'PY2'
import json, os, sys
scope = os.environ["SCOPE"].encode().decode("unicode_escape")
data = json.load(open(sys.argv[1])); data["title"] = f"Taxiway \u2013 watch agent ({scope})"
json.dump(data, open(sys.argv[1], "w"))
PY2
  expect_fail "agent title with a non-ASCII scope ($scope)" "$tree" "title must be 'Taxiway – <role> agent (<scope>)'"
  rm -rf "$tree"
done

tree="$(fixture agent-duplicate-title-case)"
sed 's/"watch"/"other"/; s/"0 4 \* \* \*"/"0 5 * * *"/; s/watch agent (fixture)/Watch agent (Fixture)/' "$tree/.agents/agents/watch.json" > "$tree/.agents/agents/other.json"
printf '# Other\n' > "$tree/.agents/agents/other.prompt.md"
expect_fail 'two agents with one title in different case' "$tree" "title 'Taxiway – watch agent (fixture)' already used by other"

tree="$(fixture agent-watchdog-title)"
sed 's/"watch"/"watchdog"/' "$tree/.agents/agents/watch.json" > "$tree/.agents/agents/watchdog.json"
rm "$tree/.agents/agents/watch.json" "$tree/.agents/agents/watch.prompt.md"
printf '# Watchdog\n' > "$tree/.agents/agents/watchdog.prompt.md"
expect_fail 'watchdog with an agent title' "$tree" "watchdog.json: title must be 'Taxiway – agents watchdog'"
sed -i.bak 's/"title": "[^"]*"/"title": "Taxiway – agents watchdog"/' "$tree/.agents/agents/watchdog.json"
bash "$checker" "$tree" > "$test_dir/out"
echo 'PASS: watchdog title accepted'

tree="$(fixture agent-missing-field)"
python3 - "$tree/.agents/agents/watch.json" <<'PY2'
import json, sys
data = json.load(open(sys.argv[1])); del data["workdir"]; data["extra"] = 1
json.dump(data, open(sys.argv[1], "w"))
PY2
expect_fail 'agent with missing and unknown fields' "$tree" "missing fields ['workdir']"

tree="$(fixture agent-wrong-id)"
sed -i.bak 's/"id": "watch"/"id": "other"/' "$tree/.agents/agents/watch.json"
expect_fail 'agent id not matching its file' "$tree" "id must be 'watch'"

tree="$(fixture agent-invalid-json)"
printf '{' > "$tree/.agents/agents/watch.json"
expect_fail 'invalid agent JSON' "$tree" '.agents/agents/watch.json: invalid JSON'

set_autonomy() {
  python3 - "$1/.agents/agents/watch.json" "$2" <<'PY2'
import json, sys
data = json.load(open(sys.argv[1])); data["autonomy"] = json.loads(sys.argv[2])
json.dump(data, open(sys.argv[1], "w"))
PY2
}

tree="$(fixture agent-autonomy-string)"
set_autonomy "$tree" '"ask"'
expect_fail 'agent autonomy as a single level' "$tree" 'autonomy must map action classes to'

tree="$(fixture agent-autonomy-unknown-class)"
set_autonomy "$tree" '{"label": "ask"}'
expect_fail 'agent autonomy with an unknown action class' "$tree" "autonomy 'label' is not an action class"

tree="$(fixture agent-autonomy-bad-level)"
set_autonomy "$tree" '{"comment-issue": "always"}'
expect_fail 'agent autonomy with an unknown level' "$tree" "autonomy 'comment-issue' must be one of"

tree="$(fixture agent-autonomy-never)"
set_autonomy "$tree" '{"push-main": "ask"}'
expect_fail 'agent autonomy with a never action' "$tree" "autonomy 'push-main' is on the never list"

tree="$(fixture agent-autonomy-never-auto)"
set_autonomy "$tree" '{"change-agent-system": "auto"}'
expect_fail 'auto for a never-auto action class' "$tree" "autonomy 'change-agent-system' can never be auto"

tree="$(fixture agent-push-without-write)"
set_autonomy "$tree" '{"push-pr-branch": "ask"}'
expect_fail 'pushing class without write on taxiway' "$tree" "['push-pr-branch'] at ask or auto requires write on taxiway-sh/taxiway"

tree="$(fixture agent-push-with-write)"
set_autonomy "$tree" '{"push-pr-branch": "auto", "change-agent-system": "ask"}'
sed -i.bak 's/"taxiway-sh\/taxiway": "read"/"taxiway-sh\/taxiway": "write"/' "$tree/.agents/agents/watch.json"
bash "$checker" "$tree" > "$test_dir/out"
echo 'PASS: write on taxiway with a pushing class accepted'

tree="$(fixture skill-merge-auto)"
sed -i.bak 's/^| `merge-pr` | Merging | `ask` |/| `merge-pr` | Merging | `auto` |/' "$tree/.agents/skills/taxiway-agents/SKILL.md"
set_autonomy "$tree" '{"merge-pr": "auto"}'
expect_fail 'never-auto class raised in the skill' "$tree" "action class 'merge-pr' must have highest level ask"
grep -qF "autonomy 'merge-pr' can never be auto" "$test_dir/out" || { echo 'FAIL: merge-pr auto accepted' >&2; exit 1; }

tree="$(fixture skill-duplicate-class)"
sed -i.bak 's/^| `merge-pr` | Merging | `ask` |/&\n| `merge-pr` | Again | `auto` |/' "$tree/.agents/skills/taxiway-agents/SKILL.md"
expect_fail 'duplicate action class row' "$tree" "duplicate Action classes row 'merge-pr'"

tree="$(fixture skill-never-list-shortened)"
sed -i.bak 's/^| `skip-tests` | Skipping tests |/| `push-tags` | Tags |/' "$tree/.agents/skills/taxiway-agents/SKILL.md"
expect_fail 'never list missing an action' "$tree" "Never list must match the checker: missing ['skip-tests'], extra ['push-tags']"

tree="$(fixture skill-never-as-class)"
sed -i.bak 's/^| `comment-issue` | Commenting |/| `skip-tests` | Skipping |/' "$tree/.agents/skills/taxiway-agents/SKILL.md"
set_autonomy "$tree" '{}'
expect_fail 'never action declared as an action class' "$tree" "'skip-tests' is on the never list, not an action class"

tree="$(fixture skill-no-action-classes)"
sed -i.bak 's/^### Action classes$/### Classes/' "$tree/.agents/skills/taxiway-agents/SKILL.md"
expect_fail 'agents skill without action classes' "$tree" 'no Action classes table'

tree="$(fixture skill-bad-highest-level)"
sed -i.bak 's/Commenting | `auto` |/Commenting | `sometimes` |/' "$tree/.agents/skills/taxiway-agents/SKILL.md"
expect_fail 'action class with an unknown highest level' "$tree" "action class 'comment-issue' highest level must be ask or auto"

tree="$(fixture agent-string-skills)"
sed -i.bak 's/"skills": \["demo"\]/"skills": "demo"/' "$tree/.agents/agents/watch.json"
expect_fail 'agent skills as a string' "$tree" 'skills must be a list of strings'

tree="$(fixture heartbeat-schema)"
printf '{"required": ["id"]}\n' > "$tree/.agents/agents/status.schema.json"
expect_fail 'heartbeat schema without promptVersion' "$tree" 'status.schema.json: must require promptVersion'

tree="$(fixture heartbeat-approvals-answers)"
sed -i.bak 's/"answer": {"enum": \["ok", "refused", "corrected"\]}/"answer": {"enum": ["ok", "refused"]}/' "$tree/.agents/agents/status.schema.json"
expect_fail 'approval log without the corrected answer' "$tree" 'status.schema.json: approvals answer must be one of'

tree="$(fixture heartbeat-approvals-list)"
python3 - "$tree/.agents/agents/status.schema.json" <<'PY2'
import json, sys
data = json.load(open(sys.argv[1])); data["properties"]["approvals"] = []
json.dump(data, open(sys.argv[1], "w"))
PY2
expect_fail 'malformed approval log' "$tree" 'status.schema.json: approvals must be an array'
if grep -q 'unreadable heartbeat schema' "$test_dir/out"; then
  echo 'FAIL: malformed approval log reported as unreadable' >&2; exit 1
fi

for target in top items; do
  tree="$(fixture "heartbeat-required-$target")"
  python3 - "$tree/.agents/agents/status.schema.json" "$target" <<'PY2'
import json, sys
data = json.load(open(sys.argv[1]))
node = data if sys.argv[2] == "top" else data["properties"]["approvals"]["items"]
node["required"] = 1
json.dump(data, open(sys.argv[1], "w"))
PY2
  if [ "$target" = top ]; then message='status.schema.json: required must be a list'; else message='status.schema.json: approvals items must require exactly'; fi
  expect_fail "heartbeat schema with a non-list required ($target)" "$tree" "$message"
  if grep -q 'Traceback' "$test_dir/out"; then echo "FAIL: non-list required ($target) crashed" >&2; exit 1; fi
done

tree="$(fixture heartbeat-approvals-fields)"
sed -i.bak 's/"required": \["class", "proposal", "answer", "date", "link"\]/"required": ["class", "answer"]/' "$tree/.agents/agents/status.schema.json"
expect_fail 'approval log missing required fields' "$tree" 'status.schema.json: approvals items must require exactly'

tree="$(fixture heartbeat-approvals-required)"
sed -i.bak 's/"waiting", "promptVersion"\]/"waiting", "promptVersion", "approvals"]/' "$tree/.agents/agents/status.schema.json"
expect_fail 'required approval log' "$tree" 'status.schema.json: approvals must stay optional'

edit_schema() {
  python3 - "$1/.agents/agents/status.schema.json" "$2" <<'PY2'
import json, sys
data = json.load(open(sys.argv[1]))
exec(sys.argv[2], {"data": data, "frictions": data["properties"]["frictions"]})
json.dump(data, open(sys.argv[1], "w"))
PY2
}

machine_agent() {
  cat > "$1/.agents/agents/machine.json" <<'JSON'
{"id": "machine", "title": "Taxiway – machine agent (Lima, macOS, real accounts)", "runs": "machine", "model": "default",
 "repositories": {"taxiway-sh/taxiway": "read", "taxiway-sh/agents": "write"},
 "workdir": "taxiway", "skills": ["demo"], "outputs": ["report"], "autonomy": {}}
JSON
  printf '# Machine\n' > "$1/.agents/agents/machine.prompt.md"
}

tree="$(fixture machine-agent)"
machine_agent "$tree"
bash "$checker" "$tree" > "$test_dir/out"
echo 'PASS: machine agent without schedule accepted'

tree="$(fixture machine-agent-schedule)"
machine_agent "$tree"
sed -i.bak 's/"runs": "machine",/"runs": "machine", "schedule": "0 5 * * *",/' "$tree/.agents/agents/machine.json"
expect_fail 'machine agent with a schedule' "$tree" 'machine.json: a machine agent is event-driven and has no schedule'

tree="$(fixture machine-agent-write-taxiway)"
machine_agent "$tree"
sed -i.bak 's/"taxiway-sh\/taxiway": "read"/"taxiway-sh\/taxiway": "write"/' "$tree/.agents/agents/machine.json"
expect_fail 'machine agent writing taxiway' "$tree" "machine.json: a machine agent's repositories must be exactly"

tree="$(fixture machine-agent-extra-repository)"
machine_agent "$tree"
sed -i.bak 's/"taxiway-sh\/agents": "write"}/"taxiway-sh\/agents": "write", "taxiway-sh\/site": "read"}/' "$tree/.agents/agents/machine.json"
expect_fail 'machine agent with an extra repository' "$tree" "machine.json: a machine agent's repositories must be exactly"

tree="$(fixture machine-agent-action-class)"
machine_agent "$tree"
sed -i.bak 's/"autonomy": {}/"autonomy": {"comment-issue": "ask"}/' "$tree/.agents/agents/machine.json"
expect_fail 'machine agent with an action class' "$tree" 'machine.json: a machine agent takes no action class; autonomy must be empty'

tree="$(fixture agent-unknown-runs)"
sed -i.bak 's/"title": "\([^"]*\)",/"title": "\1", "runs": "laptop",/' "$tree/.agents/agents/watch.json"
expect_fail 'agent with an unknown runs value' "$tree" "watch.json: runs must be one of ['cloud', 'machine']"

tree="$(fixture cloud-agent-no-schedule)"
sed -i.bak 's/"schedule": "0 4 \* \* \*", //' "$tree/.agents/agents/watch.json"
expect_fail 'cloud agent without a schedule' "$tree" 'watch.json: schedule must be five cron fields'

tree="$(fixture queue-schema-missing)"
rm "$tree/.agents/agents/queue.schema.json"
expect_fail 'missing queue schema' "$tree" 'queue.schema.json: unreadable queue schema'

set_queue_schema() {
  python3 - "$1/.agents/agents/queue.schema.json" "$2" <<'PY2'
import json, sys
data = json.load(open(sys.argv[1])); exec(sys.argv[2])
json.dump(data, open(sys.argv[1], "w"))
PY2
}

tree="$(fixture heartbeat-frictions-required)"
edit_schema "$tree" 'data["required"].append("frictions")'
expect_fail 'required frictions' "$tree" 'status.schema.json: frictions must stay optional'

tree="$(fixture heartbeat-frictions-missing)"
edit_schema "$tree" 'del data["properties"]["frictions"]'
expect_fail 'heartbeat schema without frictions' "$tree" 'status.schema.json: frictions must be an array'

tree="$(fixture heartbeat-frictions-object)"
edit_schema "$tree" 'frictions["type"] = "object"'
expect_fail 'frictions as an object' "$tree" 'status.schema.json: frictions must be an array'

tree="$(fixture heartbeat-frictions-max-items)"
edit_schema "$tree" 'del frictions["maxItems"]'
expect_fail 'frictions without an item limit' "$tree" 'status.schema.json: frictions must hold at most 20 items'

n=0
for change in 'del frictions["items"]["properties"]["summary"]["maxLength"]' \
              'del frictions["items"]["properties"]["summary"]["minLength"]' \
              'frictions["items"]["properties"]["summary"]["type"] = "array"'; do
  n=$((n + 1))
  tree="$(fixture "heartbeat-frictions-summary-$n")"
  edit_schema "$tree" "$change"
  expect_fail "frictions summary ($change)" "$tree" 'status.schema.json: frictions summary must be a non-empty string of at most 200 characters'
done

tree="$(fixture heartbeat-frictions-open-items)"
edit_schema "$tree" 'frictions["items"]["additionalProperties"] = True'
expect_fail 'frictions items with additional properties' "$tree" 'status.schema.json: frictions items must be objects without additional properties'

tree="$(fixture heartbeat-frictions-fields)"
edit_schema "$tree" 'frictions["items"]["required"] = ["kind"]'
expect_fail 'frictions items missing a required field' "$tree" "status.schema.json: frictions items must require exactly ['kind', 'summary']"

tree="$(fixture heartbeat-frictions-kinds)"
edit_schema "$tree" 'frictions["items"]["properties"]["kind"]["enum"].remove("tool-refused")'
expect_fail 'frictions without a kind' "$tree" 'status.schema.json: frictions kind must be one of'

tree="$(fixture heartbeat-frictions-extra-field)"
edit_schema "$tree" 'frictions["items"]["properties"]["session"] = {"type": "string"}'
expect_fail 'frictions with an extra field' "$tree" "status.schema.json: frictions items must have exactly ['kind', 'link', 'summary']"

tree="$(fixture heartbeat-frictions-link)"
edit_schema "$tree" 'frictions["items"]["properties"]["link"]["pattern"] = "^https://"'
expect_fail 'frictions link outside the project' "$tree" 'status.schema.json: frictions link must match'

tree="$(fixture queue-schema-status)"
set_queue_schema "$tree" 'data["properties"]["status"]["enum"].remove("expired")'
expect_fail 'queue schema without the expired status' "$tree" 'queue.schema.json: queue status must be one of'

tree="$(fixture queue-schema-rejected)"
set_queue_schema "$tree" 'data["properties"]["status"]["enum"].remove("rejected")'
expect_fail 'queue schema without the rejected status' "$tree" 'queue.schema.json: queue status must be one of'

tree="$(fixture queue-schema-date)"
set_queue_schema "$tree" 'del data["properties"]["started"]["pattern"]'
expect_fail 'queue schema with a date without the UTC pattern' "$tree" 'queue.schema.json: queue started must be a UTC date'

tree="$(fixture queue-schema-scope)"
set_queue_schema "$tree" 'del data["properties"]["checks"]["items"]["properties"]["scope"]["pattern"]'
expect_fail 'queue schema with a multi-line scope' "$tree" 'queue.schema.json: queue check scope must be one line'

tree="$(fixture queue-schema-open)"
set_queue_schema "$tree" 'del data["additionalProperties"]'
expect_fail 'queue schema allowing additional properties' "$tree" 'queue.schema.json: queue items must not allow additional properties'

tree="$(fixture queue-schema-required)"
set_queue_schema "$tree" 'data["required"].remove("expires")'
expect_fail 'queue schema without a time bound' "$tree" 'queue.schema.json: queue items must require exactly'

tree="$(fixture queue-schema-commit)"
set_queue_schema "$tree" 'data["properties"]["commit"]["pattern"] = "^[0-9a-f]{7,40}$"'
expect_fail 'queue schema accepting a short commit' "$tree" 'queue.schema.json: queue commit must be a full SHA'

tree="$(fixture agent-orphan-prompt)"
printf '# Gone\n' > "$tree/.agents/agents/gone.prompt.md"
expect_fail 'prompt without declaration' "$tree" '.agents/agents/gone.prompt.md: no matching declaration'

bash "$checker" "$repo_dir" > /dev/null
echo 'PASS: repository agent docs'

# The heartbeat schema itself, on sample heartbeats, when jsonschema is installed.
if python3 -c 'import jsonschema' 2>/dev/null; then
  python3 - "$repo_dir/.agents/agents/status.schema.json" <<'PY2'
import copy, json, sys
import jsonschema
schema = json.load(open(sys.argv[1]))
validator = jsonschema.Draft202012Validator(schema, format_checker=jsonschema.FormatChecker())
base = {"id": "triage", "date": "2026-10-21T04:20:00Z", "result": "ok", "summary": "Done.",
        "waiting": [], "promptVersion": "0" * 40}
entry = {"class": "label-issue", "proposal": "triage-20261020-1", "answer": "corrected",
         "date": "2026-10-20T09:00:00Z", "link": "https://github.com/taxiway-sh/taxiway/issues/1"}
validator.validate(base)
validator.validate(dict(base, approvals=[entry]))
validator.validate(dict(base, approvals=[dict(entry, proposal=entry["link"])]))
for change in ({"answer": "maybe"}, {"link": "https://example.invalid/1"}, {"extra": 1},
               {"link": "https://github.com/other/repo/issues/1"}, {"proposal": "label it"},
               {"proposal": "https://github.com/other/repo/issues/1"}):
    bad = dict(base, approvals=[dict(copy.deepcopy(entry), **change)])
    if validator.is_valid(bad):
        sys.exit(f"FAIL: approval entry {change} was accepted")
missing = {k: v for k, v in entry.items() if k != "answer"}
if validator.is_valid(dict(base, approvals=[missing])):
    sys.exit("FAIL: approval entry without an answer was accepted")
friction = {"kind": "tool-refused", "summary": "gh GraphQL calls were refused; used REST.",
            "link": "https://github.com/taxiway-sh/taxiway/issues/1"}
validator.validate(dict(base, frictions=[friction]))
validator.validate(dict(base, frictions=[{k: v for k, v in friction.items() if k != "link"}]))
for change in ({"kind": "annoyance"}, {"link": "https://github.com/other/repo/issues/1"},
               {"link": "https://example.invalid/1"}, {"summary": ""}, {"summary": "x" * 201},
               {"extra": 1}):
    if validator.is_valid(dict(base, frictions=[dict(friction, **change)])):
        sys.exit(f"FAIL: friction {change} was accepted")
for field in ("kind", "summary"):
    if validator.is_valid(dict(base, frictions=[{k: v for k, v in friction.items() if k != field}])):
        sys.exit(f"FAIL: friction without {field} was accepted")
if validator.is_valid(dict(base, frictions=[friction] * 21)):
    sys.exit("FAIL: more than 20 frictions were accepted")
PY2
  echo 'PASS: heartbeat schema validates the approval log and frictions'
  python3 - "$repo_dir/.agents/agents/queue.schema.json" <<'PY2'
import json, sys
import jsonschema
schema = json.load(open(sys.argv[1]))
validator = jsonschema.Draft202012Validator(schema, format_checker=jsonschema.FormatChecker())
item = {"id": "20261021-091500-steward", "link": "https://github.com/taxiway-sh/taxiway/pull/185",
        "commit": "0" * 40, "checks": [{"kind": "lima", "scope": "Up and PrepareRun on Lima"}],
        "maxMinutes": 90, "expires": "2026-10-24T09:15:00Z", "requester": "steward",
        "created": "2026-10-21T09:15:00Z", "status": "pending"}
started = {"started": "2026-10-21T10:00:00Z"}
report = "https://github.com/taxiway-sh/agents/blob/main/queue/machine/20261021-091500-steward.md"
done = dict(item, status="done", finished="2026-10-21T11:00:00Z", **started,
            result={"summary": "Lima: pass.", "link": report,
                    "outcomes": [{"kind": "lima", "outcome": "pass"}]})
real = dict(item, checks=[{"kind": "real-account", "scope": "Claude Code delegation"}],
            approval="https://github.com/taxiway-sh/taxiway/pull/185#issuecomment-1")
# approval is optional in the schema: the machine agent decides when it is needed.
valid = [item, {k: v for k, v in real.items() if k != "approval"}, dict(item, status="running", **started), done, real,
         dict(item, status="failed", finished="2026-10-21T10:05:00Z", **started,
              result={"summary": "Lima is not installed."}),
         dict(item, status="expired", finished="2026-10-24T10:00:00Z",
              result={"summary": "Expired before the machine was online."}),
         dict(item, status="rejected", finished="2026-10-21T09:20:00Z",
              result={"summary": "Rejected: the commit is not on a branch of taxiway-sh/taxiway."}),
         dict(real, status="rejected", finished="2026-10-21T09:20:00Z",
              result={"summary": "Rejected: the approval comment does not name this item."})]
invalid = {
    "extra field": dict(item, host="mac"),
    "short commit": dict(item, commit="abc1234"),
    "unknown check": dict(item, checks=[{"kind": "windows", "scope": "x"}]),
    "no checks": dict(item, checks=[]),
    "foreign link": dict(item, link="https://github.com/other/repo/pull/1"),
    "unknown status": dict(item, status="queued"),
    "pending with a result": dict(item, result={"summary": "x"}),
    "running without started": dict(item, status="running"),
    "done without outcomes": dict(done, result={"summary": "x", "link": report}),
    "done without report link": dict(done, result={"summary": "x", "outcomes": done["result"]["outcomes"]}),
    "expired after starting": dict(item, status="expired", finished="2026-10-24T10:00:00Z",
                                   result={"summary": "x"}, **started),
    "rejected after starting": dict(item, status="rejected", finished="2026-10-21T10:05:00Z",
                                    result={"summary": "x"}, **started),
    "rejected without a result": dict(item, status="rejected", finished="2026-10-21T10:05:00Z"),
    "date with an offset": dict(item, created="2026-10-21T09:15:00+02:00"),
    "date without a time": dict(item, expires="2026-10-24"),
    "date with fractions": dict(item, created="2026-10-21T09:15:00.5Z"),
    "report outside the queue": dict(done, result=dict(done["result"], link="https://example.invalid/r.md")),
    "multi-line scope": dict(item, checks=[{"kind": "lima", "scope": "Up on Lima\nthen more"}]),
}
for sample in valid:
    validator.validate(sample)
for name, sample in invalid.items():
    if validator.is_valid(sample):
        sys.exit(f"FAIL: queue item with {name} was accepted")
PY2
  echo 'PASS: queue schema validates queue items'
else
  echo 'SKIP: heartbeat and queue schema samples (python jsonschema not installed)'
fi
