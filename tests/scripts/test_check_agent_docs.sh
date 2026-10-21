#!/usr/bin/env bash
set -euo pipefail
repo_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
checker="$repo_dir/scripts/check-agent-docs.sh"
test_dir="$(mktemp -d)"
trap 'rm -rf "$test_dir"' EXIT

fixture() {
  local tree="$test_dir/$1"
  mkdir -p "$tree/.agents/skills/demo" "$tree/.claude/skills" "$tree/docs/contributing" "$tree/tests/live"
  cat > "$tree/AGENTS.md" <<'MD'
# Agents

Each rule has one owner; link to it instead of restating it.

## E2E scenarios

Do not add parallel suites or `TestE2E_` entry points.
See [testing](docs/contributing/testing.md#writing-tests), the
[skill](.agents/skills/demo/SKILL.md), [helpers](docs/contributing/testing.md#run-test_helperssh),
[emphasis](docs/contributing/testing.md#emphasis-word) and
[custom](docs/contributing/testing.md#custom-anchor).
MD
  cat > "$tree/docs/contributing/testing.md" <<'MD'
# Testing

## Writing tests

Follow the [E2E scenario rule](https://github.com/taxiway-sh/taxiway/blob/main/AGENTS.md#e2e-scenarios).
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

<a id="custom-anchor"></a>
MD
  cat > "$tree/.agents/skills/demo/SKILL.md" <<'MD'
---
name: demo
description: >-
  Use for the fixture.
---

Read [AGENTS.md](../../../AGENTS.md#e2e-scenarios).
MD
  printf '# Live\n' > "$tree/tests/live/AGENTS.md"
  ln -s ../../.agents/skills/demo "$tree/.claude/skills/demo"
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
{"id": "watch", "title": "Watch", "schedule": "0 4 * * *", "model": "default",
 "repositories": {"taxiway-sh/taxiway": "read", "taxiway-sh/agents": "write"},
 "workdir": "taxiway", "skills": ["demo"], "outputs": ["report"],
 "autonomy": {"comment-issue": "auto", "merge-pr": "ask"}}
JSON
  printf '# Watch\n' > "$tree/.agents/agents/watch.prompt.md"
  cp "$repo_dir/.agents/agents/status.schema.json" "$tree/.agents/agents/status.schema.json"
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
sed 's/"watch"/"other"/; s/"Watch"/"Other"/' "$tree/.agents/agents/watch.json" > "$tree/.agents/agents/other.json"
printf '# Other\n' > "$tree/.agents/agents/other.prompt.md"
expect_fail 'two agents on one schedule' "$tree" "schedule '0 4 * * *' already used by"

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
PY2
  echo 'PASS: heartbeat schema validates the approval log'
else
  echo 'SKIP: heartbeat schema samples (python jsonschema not installed)'
fi
