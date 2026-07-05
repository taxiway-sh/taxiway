#!/usr/bin/env bash
# tests/scripts/infra/commands/test_reset.sh - contract tests for infra/commands/reset.sh.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RESET_SH="$SCRIPT_DIR/../../../../infra/commands/reset.sh"

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

_assert_exists() {
  local test_name="$1" path="$2"
  if [[ -e "$path" ]]; then
    _pass "$test_name"
  else
    _fail "$test_name" "expected path to exist: $path"
  fi
}

_assert_missing() {
  local test_name="$1" path="$2"
  if [[ ! -e "$path" ]]; then
    _pass "$test_name"
  else
    _fail "$test_name" "expected path to be absent: $path"
  fi
}

_assert_contains() {
  local test_name="$1" haystack="$2" needle="$3"
  if [[ "$haystack" == *"$needle"* ]]; then
    _pass "$test_name"
  else
    _fail "$test_name" "missing: $needle" "in: $haystack"
  fi
}

echo "=== reset.sh confirmation ==="

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
target="$tmp/work"
mkdir -p "$target/dir"
touch "$target/file.txt" "$target/dir/nested.txt"

if printf '\n' | LAB_RESET_TARGET="$target" bash "$RESET_SH" >/dev/null 2>&1; then
  _fail "default answer aborts reset" "reset succeeded without confirmation"
else
  _pass "default answer aborts reset"
fi
_assert_exists "aborted reset preserves files" "$target/file.txt"

echo ""
echo "=== reset.sh gastown shutdown ==="

rm -rf "$target"
fake_bin="$tmp/bin"
gt_log="$tmp/gt.log"
mkdir -p "$fake_bin" "$target/gt"
cat >"$fake_bin/gt" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf 'cwd=%s gt %s\n' "$PWD" "$*" >> "$GT_LOG"
exit 0
EOF
chmod +x "$fake_bin/gt"

PATH="$fake_bin:$PATH" GT_LOG="$gt_log" LAB_RESET_TARGET="$target" LAB_RESET_YES=1 bash "$RESET_SH" >/dev/null

_assert_contains "gastown reset stops runtime before cleanup" "$(cat "$gt_log")" "cwd=$target/gt gt down --all"
_assert_missing "gastown directory removed after shutdown" "$target/gt"

echo ""
echo "=== reset.sh wipe contract ==="

rm -rf "$target"
mkdir -p "$target/dir" "$target/.cache"
touch "$target/file.txt" "$target/dir/nested.txt" "$target/.cache/state"
touch "$target/.gitkeep" "$target/.taxiway-phase"

LAB_RESET_TARGET="$target" LAB_RESET_YES=1 bash "$RESET_SH" >/dev/null

_assert_missing "regular file removed" "$target/file.txt"
_assert_missing "regular directory removed" "$target/dir"
_assert_missing "hidden non-marker directory removed" "$target/.cache"
_assert_exists ".gitkeep preserved" "$target/.gitkeep"
_assert_exists ".taxiway-* marker preserved" "$target/.taxiway-phase"

echo ""
echo "Results: $pass passed, $fail failed"
[[ $fail -eq 0 ]]
