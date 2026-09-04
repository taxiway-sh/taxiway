#!/usr/bin/env bash
# tests/scripts/infra/workspace/test_trust_mirror.sh - tests for trust-mirror.sh.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCRIPT="$SCRIPT_DIR/../../../../infra/workspace/trust-mirror.sh"

pass=0
fail=0

assert_eq() {
    local test_name="$1" got="$2" expected="$3"
    if [[ "$got" == "$expected" ]]; then
        echo "  PASS: $test_name"
        ((pass++)) || true
    else
        echo "  FAIL: $test_name"
        echo "        expected: $expected"
        echo "        got:      $got"
        ((fail++)) || true
    fi
}

assert_fails() {
    local test_name="$1"
    shift
    if ! "$@" >/dev/null 2>&1; then
        echo "  PASS: $test_name"
        ((pass++)) || true
    else
        echo "  FAIL: $test_name (expected failure)"
        ((fail++)) || true
    fi
}

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT
export HOME="$tmp_dir/home"
mkdir -p "$HOME"

echo "=== trust-mirror.sh ==="

TAXIWAY_REPO_FORK_URL='file:///lab/git/agreement-hub.git' bash "$SCRIPT" >/dev/null
TAXIWAY_REPO_FORK_URL='file:///lab/git/agreement-hub.git' bash "$SCRIPT" >/dev/null

configured_count="$(git config --global --get-all safe.directory | grep -Fxc '/lab/git/agreement-hub.git')"
assert_eq "adds the exact Taxiway mirror once" "$configured_count" "1"

git config --global --add safe.directory '/lab/git/another.git'
TAXIWAY_REPO_FORK_URL='file:///lab/git/agreement-hub.git' bash "$SCRIPT" >/dev/null
preserved_count="$(git config --global --get-all safe.directory | grep -Fxc '/lab/git/another.git')"
assert_eq "preserves existing safe directories" "$preserved_count" "1"

assert_fails "rejects a mirror outside /lab/git" \
    env TAXIWAY_REPO_FORK_URL='file:///tmp/unmanaged.git' bash "$SCRIPT"

assert_fails "rejects a non-file remote" \
    env TAXIWAY_REPO_FORK_URL='https://example.com/repo.git' bash "$SCRIPT"

echo ""
echo "Results: $pass passed, $fail failed"
[[ $fail -eq 0 ]]
