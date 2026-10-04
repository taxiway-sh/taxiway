#!/usr/bin/env bash
# Contract tests for Claude Code and Codex version pinning: install
# reconciliation, failure diagnostics and self-update policy.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"

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

_assert() {
    local test_name="$1"
    shift
    if "$@"; then
        _pass "$test_name"
    else
        _fail "$test_name" "assertion failed: $*"
    fi
}

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

# fake_lab <dir> sets up a lab HOME and a PATH with fake npm, sudo, bwrap,
# claude and codex. The fake registry publishes 1.0.0 and 2.0.0 for both
# packages; installs are recorded in $dir/installed-<bin> and $dir/npm.log.
fake_lab() {
    local dir="$1"
    mkdir -p "$dir/home" "$dir/bin"
    cat > "$dir/bin/npm" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
echo "$*" >> "$FAKE_LAB/npm.log"
spec="$(printf '%s\n' "$@" | grep -m1 '^@')"
pkg="${spec%@*}"
version="${spec##*@}"
case "$pkg" in
  @anthropic-ai/claude-code) bin=claude ;;
  @openai/codex) bin=codex ;;
  *) echo "unexpected package $pkg" >&2; exit 2 ;;
esac
case "$version" in
  latest) version=2.0.0 ;;
  1.0.0|2.0.0) ;;
  *) echo "npm error code ETARGET" >&2; echo "npm error notarget No matching version found for $spec." >&2; exit 1 ;;
esac
case "$1" in
  install) echo "${FAKE_INSTALLED_VERSION:-$version}" > "$FAKE_LAB/installed-$bin" ;;
esac
EOF
    cat > "$dir/bin/claude" <<'EOF'
#!/usr/bin/env bash
[[ -f "$FAKE_LAB/installed-claude" ]] || exit 127
case "${1:-}" in
  --version) echo "$(cat "$FAKE_LAB/installed-claude") (Claude Code)" ;;
  *) exit 0 ;;
esac
EOF
    cat > "$dir/bin/codex" <<'EOF'
#!/usr/bin/env bash
[[ -f "$FAKE_LAB/installed-codex" ]] || exit 127
case "${1:-}" in
  --version) echo "codex-cli $(cat "$FAKE_LAB/installed-codex")" ;;
  *) exit 0 ;;
esac
EOF
    printf '#!/usr/bin/env bash\nexit 0\n' > "$dir/bin/bwrap"
    printf '#!/usr/bin/env bash\nexec "$@"\n' > "$dir/bin/sudo"
    chmod +x "$dir/bin/"*
}

# The fake binaries report no version until npm installs them.
lab_run() {
    local dir="$1"
    shift
    PATH="$dir/bin:$PATH" HOME="$dir/home" FAKE_LAB="$dir" TAXIWAY_LAB=pin-lab "$@"
}

npm_installs() {
    grep -c '^install ' "$1/npm.log" 2>/dev/null || true
}

for agent in claude-code codex; do
    case "$agent" in
      claude-code) bin=claude; setting_env=TAXIWAY_SET_CLAUDE_CODE_VERSION ;;
      codex) bin=codex; setting_env=TAXIWAY_SET_CODEX_VERSION ;;
    esac
    install_sh="$REPO_ROOT/agents/$agent/install.sh"

    echo "=== $agent install ==="
    lab="$tmp_dir/$agent"
    fake_lab "$lab"

    if out="$(lab_run "$lab" env "$setting_env=1.0.0" bash "$install_sh" 2>&1)"; then
        _pass "$agent installs the pinned release in a fresh lab"
    else
        _fail "$agent installs the pinned release in a fresh lab" "$out"
    fi
    _assert "$agent installer reports its actual executable version" test "${out#*'"type":"agent-version"'}" != "$out"
    if [[ "$agent" == claude-code ]]; then
        _assert "Claude pinned installation suppresses updates in ordinary login shells" grep -qx 'DISABLE_AUTOUPDATER=1' "$lab/home/.config/taxiway/env"
    fi
    _assert "$agent npm installed the exact release" grep -q "@1.0.0$" "$lab/npm.log"

    lab_run "$lab" env "$setting_env=1.0.0" bash "$install_sh" >/dev/null 2>&1
    _assert "$agent reapplying the same pin is idempotent" test "$(npm_installs "$lab")" = 1

    lab_run "$lab" env "$setting_env=2.0.0" bash "$install_sh" >/dev/null 2>&1
    _assert "$agent changing the pin upgrades" test "$(cat "$lab/installed-$bin")" = 2.0.0
    lab_run "$lab" env "$setting_env=1.0.0" bash "$install_sh" >/dev/null 2>&1
    _assert "$agent changing the pin downgrades" test "$(cat "$lab/installed-$bin")" = 1.0.0

    out="$(lab_run "$lab" env "$setting_env=2.0.0" FAKE_INSTALLED_VERSION=1.0.0 bash "$install_sh" 2>&1)" && status=0 || status=$?
    if [[ "$status" -ne 0 && "$out" == *"requested 2.0.0"* && "$out" == *"installed 1.0.0"* ]]; then
        _pass "$agent refuses a mismatched executable after installation"
    else
        _fail "$agent refuses a mismatched executable after installation" "status=$status" "$out"
    fi

    out="$(lab_run "$lab" env "$setting_env=2.0.0" bash "$REPO_ROOT/agents/$agent/verify.sh" 2>&1)" && status=0 || status=$?
    if [[ "$status" -ne 0 && "$out" == *"requested 2.0.0"* && "$out" == *"installed 1.0.0"* ]]; then
        _pass "$agent verification rejects an executable that differs from the pin"
    else
        _fail "$agent verification rejects an executable that differs from the pin" "status=$status" "$out"
    fi
    out="$(lab_run "$lab" env "$setting_env=1.0.0" bash "$REPO_ROOT/agents/$agent/verify.sh" 2>&1)" && status=0 || status=$?
    if [[ "$status" -eq 0 && "$out" == *'"type":"agent-version"'* && "$out" == *'"actual":"1.0.0"'* && "$out" == *'"requested":"1.0.0"'* ]]; then
        _pass "$agent verification records requested and actual versions"
    else
        _fail "$agent verification records requested and actual versions" "status=$status" "$out"
    fi

    if [[ "$agent" == claude-code ]]; then
        out="$(lab_run "$lab" env "$setting_env=2.0.0" bash "$REPO_ROOT/agents/$agent/auth.sh" 2>&1)" && status=0 || status=$?
        if [[ "$status" -ne 0 && "$out" == *"version mismatch"* ]]; then
            _pass "Claude authentication refuses a different executable"
        else
            _fail "Claude authentication refuses a different executable" "status=$status" "$out"
        fi
    fi

    before="$(npm_installs "$lab")"
    lab_run "$lab" env -u "$setting_env" bash "$install_sh" >/dev/null 2>&1
    _assert "$agent unpinned keeps the installed release" test "$(npm_installs "$lab")" = "$before"
    if [[ "$agent" == claude-code ]]; then
        _assert "Claude clearing its pin removes the common shell update policy" bash -c 'test -f "$1" && ! grep -q DISABLE_AUTOUPDATER "$1"' _ "$lab/home/.config/taxiway/env"
    fi

    out="$(lab_run "$lab" env "$setting_env=9.9.9" bash "$install_sh" 2>&1)" && status=0 || status=$?
    if [[ "$status" -ne 0 && "$out" == *"npm view @"* && "$out" == *"--set ${agent}-version=<version>"* ]]; then
        _pass "$agent missing release fails with an actionable diagnostic"
    else
        _fail "$agent missing release fails with an actionable diagnostic" "status=$status" "$out"
    fi
    _assert "$agent missing release does not fall back to another version" test "$(cat "$lab/installed-$bin")" = 1.0.0

    out="$(lab_run "$lab" env "$setting_env=^1.0" bash "$install_sh" 2>&1)" && status=0 || status=$?
    if [[ "$status" -ne 0 && "$out" == *"not supported"* ]]; then
        _pass "$agent rejects ranges and dist-tags"
    else
        _fail "$agent rejects ranges and dist-tags" "status=$status" "$out"
    fi
done

echo "=== Claude Code self-update policy ==="
# shellcheck source=../../../agents/claude-code/env.sh
source "$REPO_ROOT/agents/claude-code/env.sh"
env_home="$tmp_dir/env-home"
mkdir -p "$env_home/.config/taxiway"
printf '# >>> taxiway gateway scope=gateway\nGATEWAY_FIXTURE=preserved\n# <<< taxiway gateway scope=gateway\n' > "$env_home/.config/taxiway/env"
HOME="$env_home" TAXIWAY_SET_CLAUDE_CODE_VERSION=1.0.0 claude_code_write_env true false
_assert "pinned Claude Code launches disable the autoupdater" \
    grep -qx 'export DISABLE_AUTOUPDATER=1' "$env_home/.config/taxiway/agents/claude-code.env"
HOME="$env_home" TAXIWAY_SET_CLAUDE_CODE_VERSION=latest claude_code_write_env true false
_assert "agent policy updates preserve the gateway owned environment" grep -qx GATEWAY_FIXTURE=preserved "$env_home/.config/taxiway/env"
_assert "unpinned Claude Code launches keep the default update policy" \
    bash -c "! grep -q DISABLE_AUTOUPDATER '$env_home/.config/taxiway/agents/claude-code.env'"

HOME="$env_home" TAXIWAY_SET_CLAUDE_CODE_VERSION=2.0.0 claude_code_write_env true false
out="$(lab_run "$tmp_dir/claude-code" env HOME="$env_home" bash -c 'source "$1"; claude_code_load_env' _ "$REPO_ROOT/agents/claude-code/env.sh" 2>&1)" && status=0 || status=$?
if [[ "$status" -ne 0 && "$out" == *"requested 2.0.0"* ]]; then
    _pass "Claude direct and embedded launch environments reject a different executable"
else
    _fail "Claude direct and embedded launch environments reject a different executable" "status=$status" "$out"
fi

echo "=== Codex self-update policy ==="
codex_home="$tmp_dir/codex-home"
codex_bin="$tmp_dir/codex-start-bin"
mkdir -p "$codex_home" "$codex_bin"
printf '#!/usr/bin/env bash\n[[ "${1:-}" == has-session ]] && exit 1\nexit 0\n' > "$codex_bin/tmux"
printf '#!/usr/bin/env bash\n[[ "$*" == "-p /lab/work" ]] && exit 0\nexec /bin/mkdir "$@"\n' > "$codex_bin/mkdir"
printf '#!/usr/bin/env bash\n[[ "${1:-}" == --version ]] && echo "codex-cli ${FAKE_CODEX_VERSION:-1.0.0}"\nexit 0\n' > "$codex_bin/codex"
chmod +x "$codex_bin/"*
codex_start() {
    PATH="$codex_bin:$PATH" HOME="$codex_home" TAXIWAY_LITELLM_API_KEY=test-key TAXIWAY_SET_MODEL=test-model \
        TAXIWAY_WORKSPACE_DIR="$tmp_dir" "$@" bash "$REPO_ROOT/orchestrators/codex/start.sh" >/dev/null
}
out="$(codex_start env TAXIWAY_SET_CODEX_VERSION=2.0.0 2>&1)" && status=0 || status=$?
_assert "Codex start rejects a different executable" test "$status" -ne 0
codex_start env TAXIWAY_SET_CODEX_VERSION=1.0.0
_assert "pinned Codex disables update checks" \
    grep -qx 'check_for_update_on_startup = false' "$codex_home/.codex/config.toml"
codex_start env TAXIWAY_SET_CODEX_VERSION=1.0.0
_assert "pinned Codex start does not duplicate the update setting" \
    test "$(grep -c '^check_for_update_on_startup' "$codex_home/.codex/config.toml")" = 1
codex_start env -u TAXIWAY_SET_CODEX_VERSION
_assert "clearing the Codex pin restores update checks" \
    bash -c "! grep -q check_for_update_on_startup '$codex_home/.codex/config.toml'"

echo ""
echo "Results: $pass passed, $fail failed"
[[ "$fail" -eq 0 ]]
