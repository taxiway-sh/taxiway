#!/usr/bin/env bash
# tests/scripts/test_verify_headers.sh - contract tests for verify section headers.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"

pass=0
fail=0

TMPDIR="$(mktemp -d -t taxiway-verify-headers.XXXXXX)"
trap 'rm -rf "$TMPDIR"' EXIT

BIN_DIR="$TMPDIR/bin"
HOME_DIR="$TMPDIR/home"
mkdir -p "$BIN_DIR" "$HOME_DIR"

write_fake_bin() {
  local name="$1"
  local body="$2"
  local path="$BIN_DIR/$name"
  {
    printf '#!/usr/bin/env bash\n'
    printf 'set -euo pipefail\n'
    printf '%s\n' "$body"
  } >"$path"
  chmod +x "$path"
}

write_fake_bin gt 'case "${1:-}" in version|--version) echo "gt version test";; *) echo "gt: unsupported $*" >&2; exit 1;; esac'
write_fake_bin bd 'case "${1:-}" in version|--version) echo "bd version test";; *) echo "bd: unsupported $*" >&2; exit 1;; esac'
write_fake_bin dolt 'case "${1:-}" in version|--version) echo "dolt version test";; *) echo "dolt: unsupported $*" >&2; exit 1;; esac'
write_fake_bin sqlite3 'case "${1:-}" in --version) echo "sqlite3 version test";; *) echo "sqlite3: unsupported $*" >&2; exit 1;; esac'
write_fake_bin codex 'case "${1:-}" in --version) echo "codex version test";; --help) echo "codex help test";; *) echo "codex: unsupported $*" >&2; exit 1;; esac'
write_fake_bin claude 'case "${1:-}" in --version) echo "claude version test";; --help) echo "claude help test";; config) [ "${2:-}" = "list" ] && echo "claude config test" || { echo "claude: unsupported $*" >&2; exit 1; };; *) echo "claude: unsupported $*" >&2; exit 1;; esac'

first_content_line() {
  awk 'NF && $0 !~ /^LAB_AGENT_EVENT / { print; exit }'
}

strip_ansi() {
  sed -E $'s/\x1b\\[[0-9;]*m//g'
}

assert_first_header() {
  local test_name="$1"
  local script="$2"
  local expected="$3"
  local output
  local first

  output="$(HOME="$HOME_DIR" PATH="$BIN_DIR:/usr/local/bin:/usr/bin:/bin" bash "$script" 2>&1)"
  first="$(printf '%s\n' "$output" | first_content_line | strip_ansi)"

  if [[ "$first" == *"$expected"* ]]; then
    echo "  PASS: $test_name"
    pass=$((pass + 1))
  else
    echo "  FAIL: $test_name"
    echo "        expected first content line to contain: $expected"
    echo "        first content line was: $first"
    fail=$((fail + 1))
  fi
}

echo "=== verify section headers ==="

assert_first_header \
  "gastown verify starts with binaries section" \
  "$REPO_DIR/orchestrators/gastown/verify.sh" \
  "[gastown-verify] binaries"

assert_first_header \
  "codex agent verify starts with binaries section" \
  "$REPO_DIR/agents/codex/verify.sh" \
  "[codex-agent-verify] binaries"

assert_first_header \
  "claude-code agent verify starts with binaries section" \
  "$REPO_DIR/agents/claude-code/verify.sh" \
  "[claude-code-agent-verify] binaries"

echo ""
echo "=== Results: $pass passed, $fail failed ==="
if [ "$fail" -gt 0 ]; then
  exit 1
fi
