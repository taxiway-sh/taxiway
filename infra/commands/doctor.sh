#!/usr/bin/env bash
# Report what the lab has and whether the lab is wired up correctly.
# Intended as a quick "is the lab ready?" probe — never mutates anything.

set -euo pipefail

fail=0

check() {
  local name="$1"
  local cmd="$2"
  local out
  if out="$(eval "$cmd" 2>&1)"; then
    printf '  \033[1;32mOK\033[0m   %-10s %s\n' "$name" "$out"
  else
    printf '  \033[1;31mFAIL\033[0m %-10s %s\n' "$name" "$out"
    fail=1
  fi
}

echo "Toolchain:"
check docker  'docker --version'
check node    'node --version'
check npm     'npm --version'
check python  'python3 --version'
check java    'java -version 2>&1 | head -n1'
check git     'git --version'
check jq      'jq --version'
check rg      'rg --version | head -n1'

echo
echo "For orchestrator and agent validation, run: taxiway doctor <lab>"
if [ "$fail" -eq 0 ]; then
  echo "Lab toolchain is ready."
else
  echo "Lab has issues — see FAIL entries above."
  exit 1
fi
