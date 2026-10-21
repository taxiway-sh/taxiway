#!/usr/bin/env bash
# tests/scripts/orchestrators/gascity/test_workspace.sh - contract tests for the Gas City workspace phase.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
WORKSPACE_SH="$SCRIPT_DIR/../../../../orchestrators/gascity/workspace.sh"

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

_assert_fails() {
  local test_name="$1"
  shift
  if ! "$@" >/dev/null 2>&1; then
    _pass "$test_name"
  else
    _fail "$test_name" "expected command to fail"
  fi
}

_write_fakes() {
  local fake_bin="$1"
  mkdir -p "$fake_bin"

  # Fake gc records calls and keeps registered rigs in $GC_RIGS (name|path).
  cat > "$fake_bin/gc" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf 'cwd=%s gc' "$PWD" >> "$GC_LOG"
for arg in "$@"; do
  printf ' %q' "$arg" >> "$GC_LOG"
done
printf '\n' >> "$GC_LOG"
case "$1 ${2:-}" in
  "init "*)
    mkdir -p "$2"
    printf '[workspace]\nprovider = "claude"\n' > "$2/city.toml"
    ;;
  "rig list")
    touch "$GC_RIGS"
    python3 - "$GC_RIGS" <<'PY'
import json, sys
rigs = [dict(zip(("name", "path"), line.strip().split("|"))) for line in open(sys.argv[1]) if line.strip()]
print(json.dumps({"ok": True, "rigs": rigs}))
PY
    ;;
  "rig add")
    printf '%s|%s\n' "$5" "$3" >> "$GC_RIGS"
    ;;
esac
EOF
  chmod +x "$fake_bin/gc"

  cat > "$fake_bin/git" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf 'git' >> "$GIT_LOG"
for arg in "$@"; do
  printf ' %q' "$arg" >> "$GIT_LOG"
done
printf '\n' >> "$GIT_LOG"
args=("$@")
for ((i = 0; i < ${#args[@]}; i++)); do
  if [[ "${args[$i]}" == clone ]]; then
    mkdir -p "${args[${#args[@]}-1]}/.git"
  fi
done
if [[ "$1" == config && "${2:-}" == --global && "$#" -eq 3 ]]; then
  exit 1
fi
EOF
  chmod +x "$fake_bin/git"

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

run_workspace() {
  PATH="$fake_bin:$PATH" HOME="$tmp/home" \
  GC_LOG="$tmp/gc.log" GC_RIGS="$tmp/rigs" GIT_LOG="$tmp/git.log" \
  TAXIWAY_CITY_DIR="$tmp/work/gc" \
  "$@" bash "$WORKSPACE_SH" 2>&1
}

echo "=== gascity workspace skip ==="
out="$(run_workspace env)"
_assert_contains "missing repo skips workspace phase" "$out" "No repo configured for this lab"

echo ""
echo "=== gascity workspace provisioning ==="
mkdir -p "$tmp/home"
out="$(run_workspace env \
  TAXIWAY_REPO_URL=https://github.com/example/demo-repo.git \
  TAXIWAY_REPO_FORK_URL=file:///lab/git/demo-repo.git \
  TAXIWAY_REPO_PATH=sub/dir \
  TAXIWAY_WORKSPACE_DIR="$tmp/work/demo-repo/sub/dir" \
  TAXIWAY_SET_TEMPLATE=minimal)"
gc_calls="$(cat "$tmp/gc.log")"
_assert_contains "warns that repo path does not move the rig" "$out" "--repo-path is ignored for gascity"
_assert_contains "clones the lab mirror at the repository root" "$(cat "$tmp/git.log")" "clone file:///lab/git/demo-repo.git $tmp/work/demo-repo"
_assert_contains "initializes the city non-interactively with the template" "$gc_calls" \
  "gc init $tmp/work/gc --template minimal --default-provider claude --providers claude --yes --no-start --skip-provider-readiness"
_assert_contains "adds the repository as a rig from the city" "$gc_calls" "cwd=$tmp/work/gc gc rig add $tmp/work/demo-repo --name demo-repo"

echo ""
echo "=== gascity workspace idempotency ==="
: > "$tmp/gc.log"
out="$(run_workspace env \
  TAXIWAY_REPO_URL=https://github.com/example/demo-repo.git \
  TAXIWAY_WORKSPACE_DIR="$tmp/work/demo-repo")"
gc_calls="$(cat "$tmp/gc.log")"
_assert_contains "reuses the existing city" "$out" "Gas City city already initialized"
_assert_contains "reuses the registered rig" "$out" "Rig 'demo-repo' already registered"
_assert_not_contains "does not re-run gc init" "$gc_calls" "gc init"
_assert_not_contains "does not re-add the rig" "$gc_calls" "rig add"

echo ""
echo "=== gascity workspace validation ==="
printf 'other|/elsewhere\n' >> "$tmp/rigs"
mkdir -p "$tmp/work/other/.git"
_assert_fails "rig name collision with another path fails" \
  env PATH="$fake_bin:$PATH" HOME="$tmp/home" GC_LOG="$tmp/gc.log" GC_RIGS="$tmp/rigs" GIT_LOG="$tmp/git.log" \
    TAXIWAY_CITY_DIR="$tmp/work/gc" TAXIWAY_REPO_URL=https://github.com/example/other.git \
    TAXIWAY_WORKSPACE_DIR="$tmp/work/other" bash "$WORKSPACE_SH"
_assert_fails "invalid rig name fails" \
  env PATH="$fake_bin:$PATH" HOME="$tmp/home" GC_LOG="$tmp/gc.log" GC_RIGS="$tmp/rigs" GIT_LOG="$tmp/git.log" \
    TAXIWAY_CITY_DIR="$tmp/work/gc" TAXIWAY_REPO_URL=https://github.com/example/123.git \
    TAXIWAY_WORKSPACE_DIR="$tmp/work/123" bash "$WORKSPACE_SH"

_assert_fails "unsupported template fails before gc calls" \
  env PATH="$fake_bin:$PATH" HOME="$tmp/home" GC_LOG="$tmp/gc.log" GC_RIGS="$tmp/rigs" GIT_LOG="$tmp/git.log" \
    TAXIWAY_CITY_DIR="$tmp/work/gc-custom" TAXIWAY_REPO_URL=https://github.com/example/demo-repo.git \
    TAXIWAY_WORKSPACE_DIR="$tmp/work/demo-repo" TAXIWAY_SET_TEMPLATE=custom bash "$WORKSPACE_SH"
if [[ -e "$tmp/work/gc-custom" ]]; then
  _fail "unsupported template creates no city"
else
  _pass "unsupported template creates no city"
fi

echo ""
echo "Results: $pass passed, $fail failed"
[[ $fail -eq 0 ]]
