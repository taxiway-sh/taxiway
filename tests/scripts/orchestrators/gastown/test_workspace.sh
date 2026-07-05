#!/usr/bin/env bash
# tests/scripts/orchestrators/gastown/test_workspace.sh - contract tests for Gastown workspace phase.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
WORKSPACE_SH="$SCRIPT_DIR/../../../../orchestrators/gastown/workspace.sh"

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

_assert_file() {
  local test_name="$1" path="$2"
  if [[ -e "$path" ]]; then
    _pass "$test_name"
  else
    _fail "$test_name" "expected path to exist: $path"
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

  cat > "$fake_bin/gt" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
cmd="$1"
printf 'gt %q' "$cmd" >> "$GT_LOG"
shift || true
for arg in "$@"; do
  printf ' %q' "$arg" >> "$GT_LOG"
done
printf '\n' >> "$GT_LOG"

case "$cmd" in
  install)
    mkdir -p "$1"
    ;;
  rig)
    case "${1:-}" in
      list)
        if [[ "${GT_RIG_EXISTS:-}" == "1" ]]; then
          printf '%s\n' "${TAXIWAY_RIG_NAME:-demo_rig}"
        fi
        ;;
      add)
        rig_name="$2"
        mkdir -p "$PWD/$rig_name/refinery/rig/.git"
        ;;
    esac
    ;;
  crew)
    case "${1:-}" in
      list)
        if [[ "${GT_CREW_EXISTS:-}" == "1" ]]; then
          printf '%s\n' "${TAXIWAY_CREW_NAME:-demo_crew}"
        fi
        ;;
      add)
        crew_name="$2"
        rig_name="$4"
        mkdir -p "$PWD/$rig_name/crew/$crew_name"
        ;;
    esac
    ;;
esac
EOF
  chmod +x "$fake_bin/gt"

  cat > "$fake_bin/git" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf 'git' >> "$GIT_LOG"
for arg in "$@"; do
  printf ' %q' "$arg" >> "$GIT_LOG"
done
printf '\n' >> "$GIT_LOG"

if [[ "$1" == "config" && "${2:-}" == "--global" && "${3:-}" == "user.email" && "$#" -eq 3 ]]; then
  exit 1
fi

if [[ "$1" == "-C" ]]; then
  shift 2
fi
if [[ "$1" == "remote" && "${2:-}" == "get-url" ]]; then
  printf '%s\n' "${TAXIWAY_REPO_URL:-https://github.com/example/repo.git}"
fi
EOF
  chmod +x "$fake_bin/git"
}

echo "=== gastown workspace skip ==="

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
fake_bin="$tmp/bin"
_write_fakes "$fake_bin"
out="$(
  PATH="$fake_bin:$PATH" \
  GT_LOG="$tmp/gt-skip.log" \
  GIT_LOG="$tmp/git-skip.log" \
  TAXIWAY_HQ_DIR="$tmp/hq-skip" \
  bash "$WORKSPACE_SH" 2>&1
)"
_assert_contains "missing repo skips workspace phase" "$out" "No repo configured for this lab"

echo ""
echo "=== gastown workspace validation ==="

_assert_fails "invalid rig name fails before gt calls" \
  env PATH="$fake_bin:$PATH" GT_LOG="$tmp/gt-invalid.log" GIT_LOG="$tmp/git-invalid.log" \
    TAXIWAY_REPO_URL=https://github.com/example/repo.git \
    TAXIWAY_RIG_NAME='bad-rig' \
    TAXIWAY_CREW_NAME=crew1 \
    TAXIWAY_HQ_DIR="$tmp/hq-invalid" \
    bash "$WORKSPACE_SH"

echo ""
echo "=== gastown workspace provisioning ==="

gt_log="$tmp/gt.log"
git_log="$tmp/git.log"
hq="$tmp/hq"
out="$(
  PATH="$fake_bin:$PATH" \
  GT_LOG="$gt_log" \
  GIT_LOG="$git_log" \
  TAXIWAY_REPO_URL=https://github.com/example/repo.git \
  TAXIWAY_REPO_REF=main \
  TAXIWAY_REPO_PATH=ignored/subdir \
  TAXIWAY_RIG_NAME=demo_rig \
  TAXIWAY_CREW_NAME=demo_crew \
  TAXIWAY_HQ_DIR="$hq" \
  bash "$WORKSPACE_SH" 2>&1
)"

_assert_contains "warns that repo path is ignored" "$out" "--repo-path is ignored for gastown"
_assert_contains "exports crew workspace dir" "$out" "TAXIWAY_WORKSPACE_DIR=$hq/demo_rig/crew/demo_crew"
_assert_file "creates HQ marker" "$hq/.taxiway-hq-initialized"
_assert_file "creates crew workspace" "$hq/demo_rig/crew/demo_crew"
_assert_contains "runs gt install" "$(cat "$gt_log")" "gt install $hq --git"
_assert_contains "adds rig" "$(cat "$gt_log")" "gt rig add demo_rig https://github.com/example/repo.git"
_assert_contains "adds crew" "$(cat "$gt_log")" "gt crew add demo_crew --rig demo_rig"
_assert_contains "fetches requested ref" "$(cat "$git_log")" "git -c protocol.file.allow=never -C $hq/demo_rig/refinery/rig fetch --all --prune"
_assert_contains "checks out requested ref" "$(cat "$git_log")" "git -c protocol.file.allow=never -C $hq/demo_rig/refinery/rig checkout main"

echo ""
echo "Results: $pass passed, $fail failed"
[[ $fail -eq 0 ]]
