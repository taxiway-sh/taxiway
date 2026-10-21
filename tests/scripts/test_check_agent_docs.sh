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
  mkdir -p "$tree/.agents/agents"
  cat > "$tree/.agents/agents/watch.json" <<'JSON'
{"id": "watch", "title": "Watch", "schedule": "0 4 * * *", "model": "default",
 "repositories": {"taxiway-sh/taxiway": "read", "taxiway-sh/agents": "write"},
 "workdir": "taxiway", "skills": ["demo"], "outputs": ["report"], "autonomy": "ask"}
JSON
  printf '# Watch\n' > "$tree/.agents/agents/watch.prompt.md"
  printf '{"required": ["id", "promptVersion"]}\n' > "$tree/.agents/agents/status.schema.json"
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
expect_fail 'agent with write access to taxiway' "$tree" 'taxiway-sh/taxiway access must be read'

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

tree="$(fixture agent-bad-autonomy)"
sed -i.bak 's/"autonomy": "ask"/"autonomy": {"label": "auto"}/' "$tree/.agents/agents/watch.json"
expect_fail 'agent with unsupported autonomy' "$tree" "autonomy must be one of ['ask']"

tree="$(fixture agent-string-skills)"
sed -i.bak 's/"skills": \["demo"\]/"skills": "demo"/' "$tree/.agents/agents/watch.json"
expect_fail 'agent skills as a string' "$tree" 'skills must be a list of strings'

tree="$(fixture heartbeat-schema)"
printf '{"required": ["id"]}\n' > "$tree/.agents/agents/status.schema.json"
expect_fail 'heartbeat schema without promptVersion' "$tree" 'status.schema.json: must require promptVersion'

tree="$(fixture agent-orphan-prompt)"
printf '# Gone\n' > "$tree/.agents/agents/gone.prompt.md"
expect_fail 'prompt without declaration' "$tree" '.agents/agents/gone.prompt.md: no matching declaration'

bash "$checker" "$repo_dir" > /dev/null
echo 'PASS: repository agent docs'
