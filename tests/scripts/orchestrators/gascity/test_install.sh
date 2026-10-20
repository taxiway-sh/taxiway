#!/usr/bin/env bash
# tests/scripts/orchestrators/gascity/test_install.sh - version contract tests for the Gas City install phase.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
INSTALL_SH="$SCRIPT_DIR/../../../../orchestrators/gascity/install.sh"

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

# Plans resolve versions without inspecting or changing the guest.
plan() {
  env TAXIWAY_EXECUTION_MODE=plan TAXIWAY_PLAN_INSPECTION=unavailable HOME="$(mktemp -d)" "$@" bash "$INSTALL_SH" 2>&1
}

_expect() {
  local test_name="$1" expected_status="$2" expected="$3"
  shift 3
  local out status
  set +e
  out="$(plan "$@")"
  status=$?
  set -e
  if [[ "$status" == "$expected_status" && "$out" == *"$expected"* ]]; then
    _pass "$test_name"
  else
    _fail "$test_name" "status $status, expected $expected_status and: $expected" "output: $out"
  fi
}

echo "=== gascity install versions ==="
_expect "default installs the pinned compatibility set" 0 "Installing Gas City 1.5.0"
_expect "default Beads comes from the compatibility matrix" 0 "Installing Beads 1.3.1"
_expect "default Dolt comes from the compatibility matrix" 0 "Installing Dolt 2.2.0"
_expect "v-prefixed exact versions are accepted" 0 "Installing Gas City 1.5.0" TAXIWAY_SET_VERSION=v1.5.0
_expect "latest is rejected for reproducibility" 1 "must be an exact release" TAXIWAY_SET_VERSION=latest
_expect "unmapped version requires a Beads override" 1 "set --set beads-version=<version>" TAXIWAY_SET_VERSION=9.9.9
_expect "unmapped version requires a Dolt override" 1 "set --set dolt-version=<version>" TAXIWAY_SET_VERSION=9.9.9 TAXIWAY_SET_BEADS_VERSION=1.3.1
_expect "explicit overrides install an unmapped release" 0 "Installing Dolt 2.3.0" \
  TAXIWAY_SET_VERSION=9.9.9 TAXIWAY_SET_BEADS_VERSION=1.3.1 TAXIWAY_SET_DOLT_VERSION=2.3.0

echo ""
echo "Results: $pass passed, $fail failed"
[[ $fail -eq 0 ]]
