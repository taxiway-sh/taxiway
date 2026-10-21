#!/usr/bin/env bash
# tests/scripts/orchestrators/gascity/test_stop.sh - stop hook tests for the Gas City adapter.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
STOP_SH="$SCRIPT_DIR/../../../../orchestrators/gascity/stop.sh"

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
home="$tmp/home"
fake_bin="$tmp/bin"
city="$tmp/work/gc"
mkdir -p "$home" "$fake_bin" "$city"
touch "$city/city.toml"

# Fake gc: the supervisor runs while $GC_RUNNING exists; a sticky supervisor
# refuses to stop.
cat > "$fake_bin/gc" <<'EOF'
#!/usr/bin/env bash
printf 'gc %s\n' "$*" >> "$GC_LOG"
case "$1 ${2:-}" in
  "supervisor status") [[ -f "$GC_RUNNING" ]] ;;
  "supervisor stop") [[ -n "${GC_SUPERVISOR_STICKY:-}" ]] || rm -f "$GC_RUNNING" ;;
esac
EOF
cat > "$fake_bin/pkill" <<'EOF'
#!/usr/bin/env bash
printf 'pkill %s\n' "$*" >> "$GC_LOG"
EOF
# The guest has coreutils' timeout; hosts such as macOS do not. The fakes
# return at once, so the stand-in runs the command without a bound.
cat > "$fake_bin/timeout" <<'EOF'
#!/usr/bin/env bash
shift
exec "$@"
EOF
chmod +x "$fake_bin/gc" "$fake_bin/pkill" "$fake_bin/timeout"

run_stop() {
  : > "$tmp/gc.log"
  PATH="$fake_bin:$PATH" HOME="$home" GC_LOG="$tmp/gc.log" GC_RUNNING="$tmp/running" \
    TAXIWAY_CITY_DIR="$city" "$@" bash "$STOP_SH" 2>&1
}

echo "=== gascity stop hook ==="
touch "$tmp/running"
if out="$(run_stop env)"; then
  calls="$(cat "$tmp/gc.log")"
  if [[ "$calls" == *"gc stop $city"* && "$calls" == *"gc supervisor stop --wait"* && "$calls" == *"pkill -u $(id -u) -f gc nudge poll"* && ! -f "$tmp/running" ]]; then
    _pass "stops the city, the supervisor and the nudge pollers"
  else
    _fail "stops the city, the supervisor and the nudge pollers" "$calls"
  fi
else
  _fail "stops a running city" "$out"
fi

if out="$(run_stop env)" && [[ "$(cat "$tmp/gc.log")" != *"gc stop"* ]]; then
  _pass "does not wait on a city whose supervisor is not running"
else
  _fail "does not wait on a city whose supervisor is not running" "$out" "$(cat "$tmp/gc.log")"
fi

touch "$tmp/running"
if out="$(run_stop env GC_SUPERVISOR_STICKY=1)"; then
  _fail "fails when the supervisor does not stop" "$out"
elif [[ "$out" == *"could not stop the running Gas City supervisor"* ]]; then
  _pass "fails when the supervisor does not stop"
else
  _fail "fails when the supervisor does not stop" "$out"
fi
rm -f "$tmp/running"

if out="$(run_stop env TAXIWAY_EXECUTION_MODE=plan)" && [[ ! -s "$tmp/gc.log" ]]; then
  _pass "plan mode changes nothing"
else
  _fail "plan mode changes nothing" "$out" "$(cat "$tmp/gc.log")"
fi

echo ""
echo "Results: $pass passed, $fail failed"
[[ "$fail" -eq 0 ]]
