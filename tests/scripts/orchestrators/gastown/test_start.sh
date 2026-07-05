#!/usr/bin/env bash
# tests/scripts/orchestrators/gastown/test_start.sh - contract tests for Gastown start phase.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
START_SH="$SCRIPT_DIR/../../../../orchestrators/gastown/start.sh"

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

_assert_contains() {
  local test_name="$1" output="$2" expected="$3"
  if grep -qF -- "$expected" <<<"$output"; then
    _pass "$test_name"
  else
    _fail "$test_name" "expected to find: $expected" "in output: $output"
  fi
}

_assert_not_contains() {
  local test_name="$1" output="$2" unexpected="$3"
  if grep -qF -- "$unexpected" <<<"$output"; then
    _fail "$test_name" "did not expect to find: $unexpected" "in output: $output"
  else
    _pass "$test_name"
  fi
}

_assert_order() {
  local test_name="$1" output="$2" first="$3" second="$4"
  local first_line second_line
  first_line="$(grep -nF -- "$first" <<<"$output" | head -n1 | cut -d: -f1)"
  second_line="$(grep -nF -- "$second" <<<"$output" | head -n1 | cut -d: -f1)"
  if [[ -n "$first_line" && -n "$second_line" && "$first_line" -lt "$second_line" ]]; then
    _pass "$test_name"
  else
    _fail "$test_name" "expected '$first' before '$second'" "in output: $output"
  fi
}

_assert_count() {
  local test_name="$1" output="$2" expected="$3" needle="$4"
  local count
  count="$(grep -cF -- "$needle" <<<"$output" || true)"
  if [[ "$count" == "$expected" ]]; then
    _pass "$test_name"
  else
    _fail "$test_name" "expected $expected occurrence(s) of: $needle" "found $count in output: $output"
  fi
}

_write_fakes() {
  local fake_bin="$1"
  mkdir -p "$fake_bin"

  cat > "$fake_bin/gt" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf 'gt' >> "$GT_LOG"
for arg in "$@"; do
  printf ' %q' "$arg" >> "$GT_LOG"
done
printf '\n' >> "$GT_LOG"

case "${1:-}" in
  daemon)
    case "${2:-}" in
      status)
        if [[ -f "${GT_DAEMON_STARTED_FILE:-$GT_LOG.daemon-started}" ]]; then
          printf '%s\n' "${GT_DAEMON_STATUS_AFTER_START_OUTPUT:-Daemon is running}"
          exit "${GT_DAEMON_STATUS_AFTER_START:-0}"
        fi
        printf '%s\n' "${GT_DAEMON_STATUS_OUTPUT:-Daemon is not running}"
        exit "${GT_DAEMON_STATUS:-1}"
        ;;
      start)
        touch "${GT_DAEMON_STARTED_FILE:-$GT_LOG.daemon-started}"
        exit 0
        ;;
      logs)
        if [[ -f "${GT_DAEMON_STARTED_FILE:-$GT_LOG.daemon-started}" ]]; then
          printf '%s\n' "${GT_DAEMON_LOGS_OUTPUT:-daemon logs}"
        else
          if [[ -n "${GT_DAEMON_LOGS_BEFORE_OUTPUT:-}" ]]; then
            printf '%s\n' "$GT_DAEMON_LOGS_BEFORE_OUTPUT"
          fi
        fi
        exit 0
        ;;
    esac
    ;;
  up|enable|status)
    if [[ "${1:-}" == "up" ]]; then
      exit "${GT_UP_STATUS:-0}"
    fi
    exit 0
    ;;
  crew)
    [[ "${2:-}" == "start" ]] && exit 0
    ;;
  shell)
    [[ "${2:-}" == "install" ]] && exit 0
    ;;
  doctor)
    exit 0
    ;;
esac
EOF
  chmod +x "$fake_bin/gt"

  cat > "$fake_bin/tmux" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf 'tmux' >> "$TMUX_LOG"
for arg in "$@"; do
  printf ' %q' "$arg" >> "$TMUX_LOG"
done
printf '\n' >> "$TMUX_LOG"

if [[ "${1:-}" == "has-session" ]]; then
  exit 1
fi
exit 0
EOF
  chmod +x "$fake_bin/tmux"
}

echo "=== gastown start waits for daemon heartbeat before gt up ==="

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
fake_bin="$tmp/bin"
_write_fakes "$fake_bin"

gt_log="$tmp/gt.log"
tmux_log="$tmp/tmux.log"
home="$tmp/home"
hq="$tmp/hq"
mkdir -p "$home" "$hq/.claude" "$hq/demo_rig/crew/demo_crew"
touch "$hq/.taxiway-hq-initialized"

PATH="$fake_bin:$PATH" \
HOME="$home" \
GT_LOG="$gt_log" \
TMUX_LOG="$tmux_log" \
GT_DAEMON_STATUS=1 \
GT_DAEMON_STATUS_OUTPUT="Daemon is not running" \
GT_DAEMON_LOGS_OUTPUT="Heartbeat complete (#1)" \
TAXIWAY_HQ_DIR="$hq" \
TAXIWAY_LITELLM_API_KEY="test-key" \
TAXIWAY_RIG_NAME="demo_rig" \
TAXIWAY_CREW_NAME="demo_crew" \
bash "$START_SH" >/dev/null

gt_output="$(cat "$gt_log")"
_assert_contains "checks daemon status" "$gt_output" "gt daemon status"
_assert_contains "starts daemon before gt up when heartbeat is absent" "$gt_output" "gt daemon start"
_assert_contains "checks daemon logs for heartbeat" "$gt_output" "gt daemon logs -n 1000"
_assert_contains "runs gt up" "$gt_output" "gt up"
_assert_order "starts daemon before gt up" "$gt_output" "gt daemon start" "gt up"
_assert_order "waits for heartbeat before gt up" "$gt_output" "gt daemon logs -n 1000" "gt up"
_assert_count "does not retry gt up" "$gt_output" 1 "gt up"
_assert_not_contains "does not print old retry diagnostic" "$gt_output" "Gastown startup failed on attempt"

echo "=== gastown start logs daemon output when gt up fails ==="

tmp_failure="$(mktemp -d)"
trap 'rm -rf "$tmp" "$tmp_failure"' EXIT
fake_bin_failure="$tmp_failure/bin"
_write_fakes "$fake_bin_failure"

gt_log_failure="$tmp_failure/gt.log"
tmux_log_failure="$tmp_failure/tmux.log"
stderr_failure="$tmp_failure/stderr.log"
home_failure="$tmp_failure/home"
hq_failure="$tmp_failure/hq"
mkdir -p "$home_failure" "$hq_failure/.claude" "$hq_failure/demo_rig/crew/demo_crew"
touch "$hq_failure/.taxiway-hq-initialized"

set +e
PATH="$fake_bin_failure:$PATH" \
HOME="$home_failure" \
GT_LOG="$gt_log_failure" \
TMUX_LOG="$tmux_log_failure" \
GT_UP_STATUS=1 \
GT_DAEMON_STATUS=1 \
GT_DAEMON_STATUS_OUTPUT="Daemon is not running" \
GT_DAEMON_LOGS_OUTPUT="Heartbeat complete (#1)" \
TAXIWAY_HQ_DIR="$hq_failure" \
TAXIWAY_LITELLM_API_KEY="test-key" \
TAXIWAY_RIG_NAME="demo_rig" \
TAXIWAY_CREW_NAME="demo_crew" \
bash "$START_SH" >/dev/null 2>"$stderr_failure"
status_failure=$?
set -e

gt_output_failure="$(cat "$gt_log_failure")"
stderr_output_failure="$(cat "$stderr_failure")"
if [[ "$status_failure" != "0" ]]; then
  _pass "returns non-zero when gt up fails"
else
  _fail "returns non-zero when gt up fails" "expected non-zero status"
fi
_assert_contains "runs gt up before logging failure" "$gt_output_failure" "gt up"
_assert_contains "logs daemon output after gt up failure" "$gt_output_failure" "gt daemon logs"
_assert_count "reads daemon logs for heartbeat and failure" "$gt_output_failure" 3 "gt daemon logs"
_assert_contains "prints daemon logs to stderr" "$stderr_output_failure" "Heartbeat complete"

echo ""
echo "Results: $pass passed, $fail failed"
[[ $fail -eq 0 ]]
