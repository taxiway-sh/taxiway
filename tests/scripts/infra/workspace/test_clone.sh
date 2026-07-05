#!/usr/bin/env bash
# tests/scripts/infra/workspace/test_clone.sh - unit tests for workspace-clone.sh.
#
# Run: bash tests/scripts/infra/workspace/test_clone.sh
# Exit 0 = all tests pass; non-zero = failures detected.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LIB="$SCRIPT_DIR/../../../../infra/workspace/clone.sh"

pass=0
fail=0

_assert_eq() {
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

_assert_fails() {
    local test_name="$1"
    shift
    if ! "$@" >/dev/null 2>&1; then
        echo "  PASS: $test_name (exited non-zero as expected)"
        ((pass++)) || true
    else
        echo "  FAIL: $test_name (expected non-zero exit but got 0)"
        ((fail++)) || true
    fi
}

# shellcheck source=../../../../infra/workspace/clone.sh
source "$LIB"

echo "=== _git_protocol_opts_for_url ==="

GIT_PROTOCOL_OPTS=()
_git_protocol_opts_for_url 'file:///lab/git/repo-lab-demo.git'
_assert_eq "allows Taxiway-managed lab git remotes" \
    "${GIT_PROTOCOL_OPTS[*]}" \
    "-c protocol.file.allow=always"

GIT_PROTOCOL_OPTS=()
_git_protocol_opts_for_url 'file:///tmp/repo.git'
_assert_eq "rejects arbitrary file remotes" \
    "${GIT_PROTOCOL_OPTS[*]}" \
    "-c protocol.file.allow=never"

GIT_PROTOCOL_OPTS=()
_git_protocol_opts_for_url 'https://example.com/org/repo.git'
_assert_eq "rejects file protocol for network remotes" \
    "${GIT_PROTOCOL_OPTS[*]}" \
    "-c protocol.file.allow=never"

echo ""
echo "=== workspace_clone ==="

_assert_fails "missing TAXIWAY_REPO_URL fails" \
    bash -c "source '$LIB'; TAXIWAY_WORKSPACE_DIR=/tmp/workspace; unset TAXIWAY_REPO_URL; workspace_clone"

_assert_fails "missing TAXIWAY_WORKSPACE_DIR fails" \
    bash -c "source '$LIB'; TAXIWAY_REPO_URL=https://example.com/org/repo.git; unset TAXIWAY_WORKSPACE_DIR; workspace_clone"

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT
fake_bin="$tmp_dir/bin"
mkdir -p "$fake_bin"
cat > "$fake_bin/git" <<'EOF'
#!/usr/bin/env bash
printf '%q ' "$@" >> "$GIT_LOG"
printf '\n' >> "$GIT_LOG"
case " $* " in
  *" clone "*)
    dest="${@: -1}"
    mkdir -p "$dest/.git"
    ;;
esac
EOF
chmod +x "$fake_bin/git"

workspace="$tmp_dir/work/internal-file-remote"
log="$tmp_dir/git-file-remote.log"
HOME="$tmp_dir/home" PATH="$fake_bin:$PATH" GIT_LOG="$log" \
TAXIWAY_REPO_URL='file:///lab/git/agreement-hub.git' \
TAXIWAY_WORKSPACE_DIR="$workspace" \
workspace_clone >/dev/null 2>&1

if grep -q -- "-c protocol.file.allow=always clone file:///lab/git/agreement-hub.git" "$log"; then
    echo "  PASS: workspace_clone allows Taxiway-managed file remotes"
    ((pass++)) || true
else
    echo "  FAIL: workspace_clone allows Taxiway-managed file remotes"
    echo "        git log:"
    sed 's/^/        /' "$log"
    ((fail++)) || true
fi

workspace="$tmp_dir/work/network-remote"
log="$tmp_dir/git-network-remote.log"
HOME="$tmp_dir/home" PATH="$fake_bin:$PATH" GIT_LOG="$log" \
TAXIWAY_REPO_URL='https://example.com/org/repo.git' \
TAXIWAY_WORKSPACE_DIR="$workspace" \
workspace_clone >/dev/null 2>&1

if grep -q -- "-c protocol.file.allow=never clone https://example.com/org/repo.git" "$log"; then
    echo "  PASS: workspace_clone does not modify network remotes"
    ((pass++)) || true
else
    echo "  FAIL: workspace_clone does not modify network remotes"
    echo "        git log:"
    sed 's/^/        /' "$log"
    ((fail++)) || true
fi

existing="$tmp_dir/existing/repo"
mkdir -p "$existing/.git"
log="$tmp_dir/git-existing.log"
HOME="$tmp_dir/home" PATH="$fake_bin:$PATH" GIT_LOG="$log" \
TAXIWAY_REPO_URL='file:///lab/git/agreement-hub.git' \
TAXIWAY_REPO_REF='main' \
TAXIWAY_WORKSPACE_DIR="$existing" \
workspace_clone >/dev/null 2>&1

if grep -q -- "fetch --all --prune" "$log" && grep -q -- "checkout main" "$log"; then
    echo "  PASS: existing workspace fetches and checks out requested ref"
    ((pass++)) || true
else
    echo "  FAIL: existing workspace fetches and checks out requested ref"
    echo "        git log:"
    sed 's/^/        /' "$log"
    ((fail++)) || true
fi

echo ""
echo "Results: $pass passed, $fail failed"
[[ $fail -eq 0 ]]
