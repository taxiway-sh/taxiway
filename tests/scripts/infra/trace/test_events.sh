#!/usr/bin/env bash
# tests/scripts/infra/trace/test_events.sh - unit tests for infra/trace/events.sh
#
# Run: bash tests/scripts/infra/trace/test_events.sh
# Exit 0 = all tests pass; non-zero = failures detected.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LIB="$SCRIPT_DIR/../../../../infra/trace/events.sh"

pass=0
fail=0

_assert_contains() {
  local test_name="$1"
  local output="$2"
  local expected="$3"
  if echo "$output" | grep -qF "$expected"; then
    echo "  PASS: $test_name"
    pass=$((pass + 1))
  else
    echo "  FAIL: $test_name"
    echo "        expected to find: $expected"
    echo "        in output:        $output"
    fail=$((fail + 1))
  fi
}

_assert_not_contains() {
  local test_name="$1"
  local output="$2"
  local unexpected="$3"
  if echo "$output" | grep -qF "$unexpected"; then
    echo "  FAIL: $test_name"
    echo "        did not expect to find: $unexpected"
    echo "        in output:              $output"
    fail=$((fail + 1))
  else
    echo "  PASS: $test_name"
    pass=$((pass + 1))
  fi
}

# ── Backward-compatible tests (no LAB_TRACE_ID) ──────────────────────────────

echo "=== Backward-compatibility tests ==="

# Source the library in a subshell to isolate env for each test.
out=$(env -i bash -c "source '$LIB'; lab_emit_event phase start")
_assert_contains "output starts with LAB_AGENT_EVENT" "$out" "LAB_AGENT_EVENT"
_assert_contains "type field is phase" "$out" '"type":"phase"'
_assert_contains "phase field is start" "$out" '"phase":"start"'
_assert_not_contains "no trace_id without LAB_TRACE_ID" "$out" '"trace_id"'
_assert_not_contains "no parent_span_id without env vars" "$out" '"parent_span_id"'

# Verify source field uses ID fallback.
out=$(env -i ID=myvm bash -c "source '$LIB'; lab_emit_event phase done")
_assert_contains "source uses ID" "$out" '"source":"myvm"'
_assert_not_contains "no trace_id with only ID" "$out" '"trace_id"'

# Verify source field uses LAB_SOURCE if set.
out=$(env -i LAB_SOURCE=mysrc bash -c "source '$LIB'; lab_emit_event phase done")
_assert_contains "source uses LAB_SOURCE" "$out" '"source":"mysrc"'

# ── Trace env vars are ignored ────────────────────────────────────────────────

echo ""
echo "=== Trace env var tests ==="

out=$(env -i LAB_TRACE_ID=abc123 LAB_PARENT_SPAN_ID=xyz bash -c "source '$LIB'; lab_emit_event phase start foo")
_assert_contains "LAB_AGENT_EVENT prefix still present" "$out" "LAB_AGENT_EVENT"
_assert_not_contains "trace_id ignored when LAB_TRACE_ID set" "$out" '"trace_id"'
_assert_not_contains "parent_span_id ignored when LAB_PARENT_SPAN_ID set" "$out" '"parent_span_id"'

# ── Summary ───────────────────────────────────────────────────────────────────

echo ""
echo "=== Results: $pass passed, $fail failed ==="
if [ "$fail" -gt 0 ]; then
  exit 1
fi
