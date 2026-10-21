#!/usr/bin/env bash
# tests/scripts/orchestrators/gastown/test_stop.sh - stop hook tests for the Gas Town adapter.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
STOP_SH="$SCRIPT_DIR/../../../../orchestrators/gastown/stop.sh"

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

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
fake_bin="$tmp/bin"
hq="$tmp/work/gt"
mkdir -p "$fake_bin" "$hq" "$tmp/home"
cat > "$fake_bin/gt" <<'EOF'
#!/usr/bin/env bash
printf 'cwd=%s gt %s\n' "$PWD" "$*" >> "$GT_LOG"
EOF
chmod +x "$fake_bin/gt"

run_stop() {
  : > "$tmp/gt.log"
  PATH="$fake_bin:$PATH" HOME="$tmp/home" GT_LOG="$tmp/gt.log" TAXIWAY_HQ_DIR="$hq" "$@" bash "$STOP_SH" 2>&1
}

echo "=== gastown stop hook ==="
if out="$(run_stop env)" && [[ "$(cat "$tmp/gt.log")" == *"cwd=$hq gt down --all"* ]]; then
  _pass "stops the HQ services"
else
  _fail "stops the HQ services" "$out" "$(cat "$tmp/gt.log")"
fi
if [[ -d "$hq" ]]; then
  _pass "leaves clearing the workspace to reset"
else
  _fail "leaves clearing the workspace to reset"
fi

if out="$(run_stop env TAXIWAY_HQ_DIR="$tmp/missing")" && [[ ! -s "$tmp/gt.log" ]]; then
  _pass "does nothing without an HQ"
else
  _fail "does nothing without an HQ" "$out" "$(cat "$tmp/gt.log")"
fi

if out="$(run_stop env TAXIWAY_EXECUTION_MODE=plan)" && [[ ! -s "$tmp/gt.log" ]]; then
  _pass "plan mode changes nothing"
else
  _fail "plan mode changes nothing" "$out" "$(cat "$tmp/gt.log")"
fi

echo ""
echo "Results: $pass passed, $fail failed"
[[ "$fail" -eq 0 ]]
