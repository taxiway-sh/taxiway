#!/usr/bin/env bash
set -euo pipefail
repo_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
launcher="$repo_dir/scripts/contribution-workflow.sh"
test_dir="$(mktemp -d)"
trap 'rm -rf "$test_dir"' EXIT
mkdir -p "$test_dir/bin" "$test_dir/outside"
cat > "$test_dir/bin/codex" <<'SH'
#!/usr/bin/env bash
printf '%s\n' "$PWD" > "$WORKFLOW_CAPTURE/cwd"
printf '%s\0' "$@" > "$WORKFLOW_CAPTURE/args"
exit "${WORKFLOW_EXIT:-0}"
SH
cp "$test_dir/bin/codex" "$test_dir/bin/claude"
chmod +x "$test_dir/bin/"*
export WORKFLOW_CAPTURE="$test_dir"
export PATH="$test_dir/bin:$PATH"
if [[ ! -f "$launcher" ]]; then
  echo 'FAIL: shared fresh-session launcher is missing' >&2
  exit 1
fi
cd "$test_dir/outside"
bash "$launcher" codex --status-interval 17 --triage-interval 29 --max-agents 2 --max-heavy-tests 1 -- --profile 'two words'
[[ "$(cat "$test_dir/cwd")" == "$repo_dir" ]]
python3 - "$test_dir/args" <<'PY'
import pathlib, sys
args = pathlib.Path(sys.argv[1]).read_bytes().decode().split('\0')[:-1]
assert args[:2] == ['--profile', 'two words'], args
assert len(args) == 3, args
prompt = args[-1]
assert '.agents/skills/taxiway-workflow/SKILL.md' in prompt
assert '17 seconds' in prompt and '29 seconds' in prompt
assert '2 agents' in prompt and '1 heavy' in prompt
assert 'new need or problem' in prompt and 'backlog' in prompt
PY
echo 'PASS: shared prompt, exact arguments and repository cwd'
bash "$launcher" claude -- --print
python3 - "$test_dir/args" <<'PY'
import pathlib, sys
args = pathlib.Path(sys.argv[1]).read_bytes().decode().split('\0')[:-1]
assert args[0] == '--print' and len(args) == 2, args
PY
set +e
WORKFLOW_EXIT=37 bash "$launcher" claude
status=$?
set -e
[[ "$status" == 37 ]]
echo 'PASS: child exit status propagated'
for args in 'other' 'codex --status-interval 0' 'claude --max-agents nope' 'codex --max-heavy-tests' 'codex --unknown'; do
  if bash "$launcher" $args >/dev/null 2>&1; then
    echo "FAIL: invalid invocation accepted: $args" >&2
    exit 1
  fi
done
echo 'PASS: invalid clients, settings and incomplete options rejected'
