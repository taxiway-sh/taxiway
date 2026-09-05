#!/usr/bin/env bash
# Contract tests for agent workspace-trust hooks.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
CLAUDE_HOOK="$REPO_ROOT/agents/claude-code/trust-workspace.sh"
CODEX_HOOK="$REPO_ROOT/agents/codex/trust-workspace.sh"

pass=0
fail=0

_pass() {
    echo "  PASS: $1"
    ((pass++)) || true
}

_fail() {
    echo "  FAIL: $1"
    shift
    for line in "$@"; do
        echo "        $line"
    done
    ((fail++)) || true
}

_assert_hook_runs() {
    local test_name="$1" hook="$2" home="$3" workspace="$4"
    if HOME="$home" TAXIWAY_WORKSPACE_TRUST_PATH="$workspace" bash "$hook" >/dev/null 2>&1; then
        _pass "$test_name"
    else
        _fail "$test_name" "hook failed: $hook"
    fi
}

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

echo "=== Claude Code workspace trust ==="

claude_empty_home="$tmp_dir/claude-empty-home"
mkdir -p "$claude_empty_home"
_assert_hook_runs "Claude hook creates missing config" \
    "$CLAUDE_HOOK" "$claude_empty_home" "/lab/work"
if jq -e '.projects["/lab/work"].hasTrustDialogAccepted == true' \
    "$claude_empty_home/.claude.json" >/dev/null 2>&1; then
    _pass "Claude hook creates valid trust config"
else
    _fail "Claude hook creates valid trust config"
fi

claude_home="$tmp_dir/claude-home"
claude_workspace="$tmp_dir/work/repo-\"quoted"
mkdir -p "$claude_home" "$claude_workspace"
cat > "$claude_home/.claude.json" <<'EOF'
{
  "theme": "dark",
  "projects": {
    "/existing/project": {
      "allowedTools": ["Read"]
    }
  }
}
EOF

_assert_hook_runs "Claude hook merges trust into existing config" \
    "$CLAUDE_HOOK" "$claude_home" "$claude_workspace"

if jq -e --arg path "$claude_workspace" \
    '.theme == "dark" and .projects["/existing/project"].allowedTools == ["Read"] and .projects[$path].hasTrustDialogAccepted == true' \
    "$claude_home/.claude.json" >/dev/null 2>&1; then
    _pass "Claude hook preserves config and escapes the workspace path"
else
    _fail "Claude hook preserves config and escapes the workspace path"
fi

claude_before="$tmp_dir/claude-before.json"
cp "$claude_home/.claude.json" "$claude_before"
_assert_hook_runs "Claude hook can be run repeatedly" \
    "$CLAUDE_HOOK" "$claude_home" "$claude_workspace"
if cmp -s "$claude_before" "$claude_home/.claude.json"; then
    _pass "Claude hook is idempotent"
else
    _fail "Claude hook is idempotent"
fi
if HOME="$claude_home" TAXIWAY_WORKSPACE_TRUST_PATH="relative/path" \
    bash "$CLAUDE_HOOK" >/dev/null 2>&1; then
    _fail "Claude hook rejects relative paths"
else
    _pass "Claude hook rejects relative paths"
fi

echo ""
echo "=== Codex workspace trust ==="

codex_empty_home="$tmp_dir/codex-empty-home"
mkdir -p "$codex_empty_home"
_assert_hook_runs "Codex hook creates missing config" \
    "$CODEX_HOOK" "$codex_empty_home" "/lab/work"
if python3 - "$codex_empty_home/.codex/config.toml" <<'PY'
import sys
import tomllib

with open(sys.argv[1], "rb") as config_file:
    config = tomllib.load(config_file)

assert config["projects"]["/lab/work"]["trust_level"] == "trusted"
PY
then
    _pass "Codex hook creates valid trust config"
else
    _fail "Codex hook creates valid trust config"
fi

codex_home="$tmp_dir/codex-home"
codex_workspace="$tmp_dir/work/repo-🚀-\"quoted"
mkdir -p "$codex_home/.codex" "$codex_workspace"
cat > "$codex_home/.codex/config.toml" <<'EOF'
model = "gpt-test"

[projects."/existing/project"]
trust_level = "trusted"
marker = "preserve-me"
EOF
toml_codex_workspace="${codex_workspace//\\/\\\\}"
toml_codex_workspace="${toml_codex_workspace//\"/\\\"}"
printf '\n[projects."%s"]\nmarker = "preserve-target"\n' \
    "$toml_codex_workspace" >> "$codex_home/.codex/config.toml"

_assert_hook_runs "Codex hook merges trust into existing config" \
    "$CODEX_HOOK" "$codex_home" "$codex_workspace"

if python3 - "$codex_home/.codex/config.toml" "$codex_workspace" <<'PY'
import sys
import tomllib

with open(sys.argv[1], "rb") as config_file:
    config = tomllib.load(config_file)

assert config["model"] == "gpt-test"
assert config["projects"]["/existing/project"]["marker"] == "preserve-me"
assert config["projects"][sys.argv[2]]["marker"] == "preserve-target"
assert config["projects"][sys.argv[2]]["trust_level"] == "trusted"
PY
then
    _pass "Codex hook preserves config and escapes the workspace path"
else
    _fail "Codex hook preserves config and escapes the workspace path"
fi

codex_before="$tmp_dir/codex-before.toml"
cp "$codex_home/.codex/config.toml" "$codex_before"
_assert_hook_runs "Codex hook can be run repeatedly" \
    "$CODEX_HOOK" "$codex_home" "$codex_workspace"
if cmp -s "$codex_before" "$codex_home/.codex/config.toml"; then
    _pass "Codex hook is idempotent"
else
    _fail "Codex hook is idempotent"
fi
if HOME="$codex_home" TAXIWAY_WORKSPACE_TRUST_PATH="relative/path" \
    bash "$CODEX_HOOK" >/dev/null 2>&1; then
    _fail "Codex hook rejects relative paths"
else
    _pass "Codex hook rejects relative paths"
fi

if python3 - "$CODEX_HOOK" "$tmp_dir" <<'PY'
import os
from pathlib import Path
import subprocess
import sys
import tomllib

hook, root = sys.argv[1:]
for index, header in enumerate([
    '[projects."/lab/work"] # keep comment',
    "  [ projects . '/lab/work' ]  ",
    '[projects."/lab/\\u0077ork"]',
]):
    config_dir = Path(root) / f"codex-format-{index}" / ".codex"
    config_dir.mkdir(parents=True)
    config = config_dir / "config.toml"
    original = header + '\nnotes = """\n[not_a_table]\ntrust_level = "not_a_setting"\n"""\n"trust_level" = "untrusted"\nmarker = "keep"\n  [other]\nvalue = "keep"\n'
    config.write_text(original)
    env = dict(os.environ, HOME=str(config_dir.parent), TAXIWAY_WORKSPACE_TRUST_PATH="/lab/work")
    subprocess.run(["bash", hook], env=env, check=True)
    result = config.read_text()
    parsed = tomllib.loads(result)
    expected = tomllib.loads(original)
    expected["projects"]["/lab/work"]["trust_level"] = "trusted"
    assert parsed == expected
    assert header in result
    subprocess.run(["bash", hook], env=env, check=True)
    assert config.read_text() == result
    config.write_text("invalid = [")
    failed = subprocess.run(["bash", hook], env=env, capture_output=True)
    assert failed.returncode != 0
    assert config.read_text() == "invalid = ["
PY
then
    _pass "Codex hook preserves alternate TOML headers and is idempotent"
else
    _fail "Codex hook preserves alternate TOML headers and is idempotent"
fi

echo ""
echo "Results: $pass passed, $fail failed"
[[ $fail -eq 0 ]]
