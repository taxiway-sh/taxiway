#!/usr/bin/env bash
# Contract tests for Claude Code's read-only verification command.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
VERIFY_SH="$SCRIPT_DIR/../../../agents/claude-code/verify.sh"

tmp_dir="$(mktemp -d -t taxiway-claude-verify.XXXXXX)"
trap 'rm -rf "$tmp_dir"' EXIT

fake_bin="$tmp_dir/bin"
mkdir -p "$fake_bin"

cat >"$fake_bin/claude" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

printf '%s\n' "$*" >>"$CLAUDE_TEST_LOG"

case "$*" in
    "--version")
        echo "2.1.260 (Claude Code)"
        ;;
    "--help")
        echo "Usage: claude [options] [command]"
        ;;
    "auth status --json")
        case "$CLAUDE_TEST_STATE" in
            authenticated)
                echo '{"loggedIn":true,"authMethod":"claude.ai","apiProvider":"firstParty"}'
                ;;
            unauthenticated)
                echo '{"loggedIn":false,"authMethod":"none","apiProvider":"none"}'
                exit 1
                ;;
            malformed)
                echo 'not json'
                exit 1
                ;;
            incomplete)
                echo '{"loggedIn":false}'
                exit 1
                ;;
            unexpected-exit)
                echo '{"loggedIn":false,"authMethod":"none","apiProvider":"none"}'
                exit 2
                ;;
        esac
        ;;
    *)
        mkdir -p "$(dirname "$CLAUDE_TEST_TRANSCRIPT")"
        : >"$CLAUDE_TEST_TRANSCRIPT"
        echo "Not logged in - Please run /login"
        exit 1
        ;;
esac
EOF
chmod +x "$fake_bin/claude"

pass=0
fail=0

record_pass() {
    echo "  PASS: $1"
    pass=$((pass + 1))
}

record_fail() {
    echo "  FAIL: $1"
    fail=$((fail + 1))
}

run_case() {
    local state="$1"
    local expected_status="$2"
    local home="$tmp_dir/home-$state"
    local log="$tmp_dir/$state.log"
    local output="$tmp_dir/$state.out"
    local transcript="$home/.claude/projects/test/transcript.jsonl"
    local status

    mkdir -p "$home"
    : >"$log"

    set +e
    HOME="$home" \
        PATH="$fake_bin:/usr/local/bin:/usr/bin:/bin" \
        CLAUDE_TEST_LOG="$log" \
        CLAUDE_TEST_STATE="$state" \
        CLAUDE_TEST_TRANSCRIPT="$transcript" \
        bash "$VERIFY_SH" >"$output" 2>&1
    status=$?
    set -e

    if [ "$status" -eq "$expected_status" ]; then
        record_pass "$state returns status $expected_status"
    else
        record_fail "$state returns status $expected_status (got $status)"
    fi

    if grep -Fxq 'auth status --json' "$log"; then
        record_pass "$state uses auth status --json"
    else
        record_fail "$state uses auth status --json"
    fi

    if ! grep -Fxq 'config list' "$log"; then
        record_pass "$state does not use config list"
    else
        record_fail "$state does not use config list"
    fi

    if [ ! -e "$transcript" ]; then
        record_pass "$state does not create a transcript"
    else
        record_fail "$state does not create a transcript"
    fi
}

echo "=== Claude Code verification ==="

run_case authenticated 0
run_case unauthenticated 0
run_case malformed 1
run_case incomplete 1
run_case unexpected-exit 1

echo ""
echo "=== Results: $pass passed, $fail failed ==="
[ "$fail" -eq 0 ]
