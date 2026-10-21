#!/usr/bin/env bash
# tests/scripts/orchestrators/gascity/test_start.sh - contract tests for the Gas City start phase.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
START_SH="$SCRIPT_DIR/../../../../orchestrators/gascity/start.sh"

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

_assert_line() {
  local test_name="$1" output="$2" expected="$3"
  if grep -qxF -- "$expected" <<<"$output"; then
    _pass "$test_name"
  else
    _fail "$test_name" "expected the exact line: $expected" "in output: $output"
  fi
}

_assert_order() {
  local test_name="$1" output="$2" first="$3" second="$4"
  local first_line second_line
  first_line="$(grep -nF -- "$first" <<<"$output" | head -n1 | cut -d: -f1)"
  second_line="$(grep -nF -- "$second" <<<"$output" | tail -n1 | cut -d: -f1)"
  if [[ -n "$first_line" && -n "$second_line" && "$first_line" -lt "$second_line" ]]; then
    _pass "$test_name"
  else
    _fail "$test_name" "expected '$first' before '$second'" "in output: $output"
  fi
}

_write_fakes() {
  local fake_bin="$1"
  mkdir -p "$fake_bin"

  # Fake gc: the supervisor runs once `gc start` was called; session state
  # comes from $GC_SESSION_STATE.
  cat > "$fake_bin/gc" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf 'gc' >> "$GC_LOG"
for arg in "$@"; do
  printf ' %q' "$arg" >> "$GC_LOG"
done
printf '\n' >> "$GC_LOG"
case "$1 ${2:-}" in
  "init "*)
    mkdir -p "$2"
    printf '[workspace]\nprovider = "claude"\n\n[providers]\n[providers.claude]\nbase = "builtin:claude"\nready_delay_ms = 0\n' > "$2/city.toml"
    ;;
  "supervisor status")
    [[ -f "$GC_RUNNING" ]]
    ;;
  "supervisor stop")
    if [[ -n "${GC_SUPERVISOR_STICKY:-}" ]]; then
      echo "fake: supervisor refused to stop" >&2
      exit 1
    fi
    rm -f "$GC_RUNNING"
    ;;
  "supervisor start")
    printf 'detached GC_SUPERVISOR_ENV=%s TAXIWAY_LITELLM_API_KEY=%s\n' "${GC_SUPERVISOR_ENV:-}" "${TAXIWAY_LITELLM_API_KEY:+set}" >> "$GC_LOG"
    touch "$GC_RUNNING"
    ;;
  "start "*)
    printf 'GC_SUPERVISOR_ENV=%s\n' "${GC_SUPERVISOR_ENV:-}" >> "$GC_LOG"
    printf 'ANTHROPIC_AUTH_TOKEN=%s\n' "${ANTHROPIC_AUTH_TOKEN:+set}" >> "$GC_LOG"
    touch "$GC_RUNNING"
    ;;
  "session list")
    printf '{"ok":true,"sessions":[{"state":"%s","session_name":"gastown__mayor"}]}\n' "${GC_SESSION_STATE:-active}"
    ;;
  "rig list")
    printf '{"ok":true,"rigs":[{"name":"demo","path":"%s"}]}\n' "${GC_RIG_PATH:-}"
    ;;
esac
EOF
  chmod +x "$fake_bin/gc"

  cat > "$fake_bin/tmux" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf 'tmux' >> "$TMUX_LOG"
for arg in "$@"; do
  printf ' %q' "$arg" >> "$TMUX_LOG"
done
printf '\n' >> "$TMUX_LOG"
# Interleave session closes with gc calls to check their relative order.
if [[ "${1:-}" == kill-session ]]; then
  printf 'tmux kill-session\n' >> "${GC_LOG:-/dev/null}"
fi
[[ "${1:-}" != has-session ]]
EOF
  chmod +x "$fake_bin/tmux"

  cat > "$fake_bin/pkill" <<'EOF'
#!/usr/bin/env bash
printf 'pkill %s\n' "$*" >> "$GC_LOG"
exit 1
EOF
  chmod +x "$fake_bin/pkill"

  # The guest has coreutils' timeout; hosts such as macOS do not. The fakes
  # return at once, so the stand-in runs the command without a bound.
  cat > "$fake_bin/timeout" <<'EOF'
#!/usr/bin/env bash
shift
exec "$@"
EOF
  chmod +x "$fake_bin/timeout"
}

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
fake_bin="$tmp/bin"
_write_fakes "$fake_bin"
home="$tmp/home"
city="$tmp/work/gc"
rig="$tmp/work/demo"
mkdir -p "$home/.config/taxiway" "$rig/.git"
printf "TAXIWAY_LITELLM_BASE_URL='http://demo.litellm.internal:4000/'\nTAXIWAY_LITELLM_API_KEY='sk-test-gateway'\n" > "$home/.config/taxiway/env"

run_start() {
  PATH="$fake_bin:$PATH" HOME="$home" \
  GC_LOG="$tmp/gc.log" GC_RUNNING="$tmp/running" TMUX_LOG="$tmp/tmux.log" \
  TAXIWAY_CITY_DIR="$city" TAXIWAY_LAB=demo \
  TAXIWAY_REPO_URL=https://github.com/example/demo.git TAXIWAY_WORKSPACE_DIR="$rig" \
  TAXIWAY_SET_MODEL=test-selected-claude-model \
  ANTHROPIC_DEFAULT_OPUS_MODEL=test-opus \
  TAXIWAY_GASCITY_READY_TIMEOUT_SECONDS=4 \
  "$@" bash "$START_SH" 2>&1
}

echo "=== gascity start configures the gateway and starts the city ==="
out="$(run_start env GC_RIG_PATH= TAXIWAY_SET_AUTH_MODE=api-key)"
gc_calls="$(cat "$tmp/gc.log")"
_assert_contains "creates the missing city" "$gc_calls" "gc init $city --template gastown"
_assert_contains "registers the workspace rig" "$gc_calls" "gc rig add $rig --name demo"
_assert_not_contains "does not stop a supervisor that is not running" "$gc_calls" "gc stop"
_assert_line "opts the gateway variables into the supervisor environment" "$gc_calls" \
  "GC_SUPERVISOR_ENV=TAXIWAY_LITELLM_API_KEY TAXIWAY_LITELLM_BASE_URL"
_assert_contains "api-key mode exports the native gateway credential" "$gc_calls" "ANTHROPIC_AUTH_TOKEN=set"
_assert_contains "waits for agent sessions" "$out" "Gas City agent sessions are active"
_assert_contains "opens the workspace shell session" "$(cat "$tmp/tmux.log")" "tmux new-session -d -s gascity -c $rig"
_assert_order "closes the previous shell session before starting the city" "$gc_calls" "tmux kill-session" "gc start $city"
if python3 - "$city" "$home" "$rig" <<'PY'
import json, pathlib, stat, sys, tomllib
city, home, rig = map(pathlib.Path, sys.argv[1:])
fragment = city / "taxiway.toml"
assert stat.S_IMODE(fragment.stat().st_mode) == 0o600
text = fragment.read_text()
assert "sk-test-gateway" not in text, "gateway key must stay a $VAR reference"
data = tomllib.loads(text)
upstream = data["upstreams"]["taxiway-litellm"]
assert upstream["base_url"] == "http://demo.litellm.internal:4000"
assert upstream["base_url_env"] == "ANTHROPIC_BASE_URL"
assert upstream["auth_token"] == "$TAXIWAY_LITELLM_API_KEY"
assert upstream["auth_token_env"] == "ANTHROPIC_AUTH_TOKEN"
assert upstream["env"]["ANTHROPIC_CUSTOM_HEADERS"] == "x-litellm-api-key: Bearer $TAXIWAY_LITELLM_API_KEY"
assert data["agent_defaults"]["upstream"] == "taxiway-litellm"
claude = data["providers"]["claude"]
assert claude["args"] == ["--model", "test-selected-claude-model"]
assert claude["env"]["ENABLE_TOOL_SEARCH"] == "true"
assert claude["env"]["ENABLE_CLAUDEAI_MCP_SERVERS"] == "false"
assert claude["env"]["ANTHROPIC_DEFAULT_OPUS_MODEL"] == "test-opus"
config = tomllib.loads((city / "city.toml").read_text())
assert config["include"] == ["taxiway.toml"]
assert config["providers"]["claude"] == {"base": "builtin:claude", "ready_delay_ms": 0, "option_defaults": {"effort": "max"}}
projects = json.loads((home / ".claude.json").read_text())["projects"]
for root in (city, rig):
    assert projects[str(root)]["hasTrustDialogAccepted"] is True
PY
then
  _pass "fragment routes Claude through the gateway with the selected model and default effort, city and rig trusted"
else
  _fail "fragment routes Claude through the gateway with the selected model and default effort, city and rig trusted"
fi

echo ""
echo "=== gascity restart is idempotent and applies subscription mode ==="
: > "$tmp/gc.log"
# Gas City rewrites city.toml in its own layout: a commented include and the
# option defaults as a subtable must both be handled.
cat > "$city/city.toml" <<'EOF'
include = ["other.toml"] # operator fragment

[workspace]
provider = "claude"

[providers]
[providers.claude]
base = "builtin:claude"

[providers.claude.option_defaults]
effort = "max"
permission_mode = "unrestricted"

[daemon]
patrol_interval = "30s"
EOF
chmod 640 "$city/city.toml"
out="$(run_start env GC_RIG_PATH="$rig" TAXIWAY_SET_AUTH_MODE=subscription ANTHROPIC_AUTH_TOKEN=stale TAXIWAY_SET_TOOL_SEARCH=false TAXIWAY_SET_EFFORT=high)"
gc_calls="$(cat "$tmp/gc.log")"
_assert_not_contains "reuses the existing city" "$gc_calls" "gc init"
_assert_not_contains "reuses the registered rig" "$gc_calls" "rig add"
_assert_order "stops the running city before restarting the supervisor" "$gc_calls" "gc stop $city" "gc start $city"
_assert_order "stops the supervisor before starting it again" "$gc_calls" "gc supervisor stop --wait" "gc start $city"
_assert_contains "subscription mode clears an inherited native credential" "$gc_calls" "ANTHROPIC_AUTH_TOKEN="
_assert_not_contains "subscription mode never forwards the gateway key as a token" "$gc_calls" "ANTHROPIC_AUTH_TOKEN=set"
if python3 - "$city" <<'PY'
import pathlib, sys, tomllib
city = pathlib.Path(sys.argv[1])
data = tomllib.loads((city / "taxiway.toml").read_text())
upstream = data["upstreams"]["taxiway-litellm"]
assert "auth_token" not in upstream and "auth_token_env" not in upstream
assert data["providers"]["claude"]["env"]["ENABLE_TOOL_SEARCH"] == "false"
text = (city / "city.toml").read_text()
config = tomllib.loads(text)
assert config["include"] == ["other.toml", "taxiway.toml"]
assert "# operator fragment" in text
assert config["providers"]["claude"]["option_defaults"] == {"effort": "high", "permission_mode": "unrestricted"}
assert config["daemon"] == {"patrol_interval": "30s"}
assert (city / "city.toml").stat().st_mode & 0o777 == 0o640
assert not list(city.glob(".taxiway-*")), "temporary files must not remain"
PY
then
  _pass "restart rewrites the fragment, keeps a single include and updates only the effort"
else
  _fail "restart rewrites the fragment, keeps a single include and updates only the effort"
fi

echo ""
echo "=== gascity start reports an include it cannot extend ==="
printf 'include = [\n  "other.toml",\n]\n%s\n' "$(sed '1d' "$city/city.toml")" > "$city/city.toml"
cp "$city/city.toml" "$tmp/city.before"
set +e
out="$(run_start env GC_RIG_PATH="$rig")"
status=$?
set -e
if [[ "$status" != 0 ]] && cmp -s "$city/city.toml" "$tmp/city.before"; then
  _pass "multi-line include fails without changing city.toml"
else
  _fail "multi-line include fails without changing city.toml" "status=$status"
fi
_assert_contains "names the include to add" "$out" 'include is not a single-line array; add "taxiway.toml" to it'
printf 'include = ["other.toml", "taxiway.toml"]\n%s\n' "$(sed '1,3d' "$city/city.toml")" > "$city/city.toml"

echo ""
echo "=== gascity start validates template and effort ==="
for setting in TAXIWAY_SET_TEMPLATE=custom TAXIWAY_SET_EFFORT=extreme; do
  set +e
  out="$(run_start env GC_RIG_PATH="$rig" "$setting")"
  status=$?
  set -e
  if [[ "$status" != 0 && "$out" == *"unsupported Gas City"* ]]; then
    _pass "rejects $setting"
  else
    _fail "rejects $setting" "status=$status" "$out"
  fi
done

echo ""
echo "=== gascity start runs the supervisor detached in a container guest ==="
: > "$tmp/gc.log"
out="$(run_start env GC_RIG_PATH="$rig" TAXIWAY_GUEST_KIND=container TAXIWAY_DASHBOARD_HOST_PORT=23456)"
gc_calls="$(cat "$tmp/gc.log")"
_assert_not_contains "does not announce a dashboard Docker does not publish" "$out" "Dashboard: http://127.0.0.1:23456"
_assert_order "stops the running supervisor before starting it detached" "$gc_calls" "gc supervisor stop --wait" "gc supervisor start"
_assert_order "starts the detached supervisor before the city" "$gc_calls" "gc supervisor start" "gc start $city"
_assert_line "the detached supervisor inherits the gateway key without a service opt-in" "$gc_calls" \
  "detached GC_SUPERVISOR_ENV= TAXIWAY_LITELLM_API_KEY=set"
_assert_line "no service environment opt-in for the city start" "$gc_calls" "GC_SUPERVISOR_ENV="
_assert_contains "explains gc's systemd message in a container" "$out" "expected in a container guest"
_assert_contains "reports the container runtime" "$out" "Starting the Gas City supervisor detached (container guest)"

echo ""
echo "=== gascity start uses gc start alone in a VM guest ==="
: > "$tmp/gc.log"
out="$(run_start env GC_RIG_PATH="$rig" TAXIWAY_GUEST_KIND=vm TAXIWAY_DASHBOARD_HOST_PORT=23456)"
gc_calls="$(cat "$tmp/gc.log")"
_assert_contains "announces the forwarded dashboard" "$out" "Dashboard: http://127.0.0.1:23456"
_assert_not_contains "does not start a detached supervisor" "$gc_calls" "gc supervisor start"
_assert_line "opts the gateway variables into the service unit" "$gc_calls" "GC_SUPERVISOR_ENV=TAXIWAY_LITELLM_API_KEY TAXIWAY_LITELLM_BASE_URL"

echo ""
echo "=== gascity start refuses a supervisor that does not stop ==="
for kind in container vm; do
  touch "$tmp/running"
  : > "$tmp/gc.log"
  set +e
  out="$(run_start env GC_RIG_PATH="$rig" TAXIWAY_GUEST_KIND="$kind" GC_SUPERVISOR_STICKY=1)"
  status=$?
  set -e
  gc_calls="$(cat "$tmp/gc.log")"
  if [[ "$status" != 0 ]]; then
    _pass "fails when the supervisor survives the retry ($kind)"
  else
    _fail "fails when the supervisor survives the retry ($kind)"
  fi
  _assert_contains "reports the stuck supervisor ($kind)" "$out" "could not stop the running Gas City supervisor"
  _assert_contains "shows the stop error ($kind)" "$out" "fake: supervisor refused to stop"
  if [[ "$(grep -cxF 'gc supervisor stop --wait' <<<"$gc_calls")" == 2 ]]; then
    _pass "retries the bounded stop once ($kind)"
  else
    _fail "retries the bounded stop once ($kind)" "$gc_calls"
  fi
  _assert_not_contains "never reuses the stale supervisor ($kind)" "$gc_calls" "gc start $city"
done
rm -f "$tmp/running"

echo ""
echo "=== gascity start fails when sessions never become active ==="
set +e
out="$(run_start env GC_RIG_PATH="$rig" GC_SESSION_STATE=creating)"
status=$?
set -e
if [[ "$status" != 0 ]]; then
  _pass "returns non-zero after the bounded readiness timeout"
else
  _fail "returns non-zero after the bounded readiness timeout"
fi
_assert_contains "reports the readiness timeout" "$out" "timed out after 4s waiting for Gas City agent sessions"

echo ""
echo "=== gascity start rejects values Gas City would expand ==="
set +e
out="$(run_start env GC_RIG_PATH="$rig" 'TAXIWAY_SET_MODEL=claude-$HOME')"
status=$?
set -e
if [[ "$status" != 0 ]]; then
  _pass "model containing a \$ reference is rejected"
else
  _fail "model containing a \$ reference is rejected" "$out"
fi

echo ""
echo "=== gascity start requires the gateway ==="
set +e
out="$(PATH="$fake_bin:$PATH" HOME="$tmp/empty-home" GC_LOG="$tmp/gc.log" GC_RUNNING="$tmp/running" TMUX_LOG="$tmp/tmux.log" \
  TAXIWAY_CITY_DIR="$city" TAXIWAY_SET_MODEL=m bash "$START_SH" 2>&1)"
status=$?
set -e
if [[ "$status" != 0 ]]; then
  _pass "missing gateway key fails"
else
  _fail "missing gateway key fails"
fi
_assert_contains "points to taxiway gateway" "$out" "taxiway gateway"

echo ""
echo "Results: $pass passed, $fail failed"
[[ $fail -eq 0 ]]
