#!/usr/bin/env bash
# tests/scripts/test_standalone_install.sh - contract tests for root install.sh defaults.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
INSTALL_SH="$SCRIPT_DIR/../../install.sh"
REPO_ROOT="$SCRIPT_DIR/../.."
GORELEASER_YAML="$REPO_ROOT/.goreleaser.yaml"
RENDER_RELEASE_INSTALL_SH="$REPO_ROOT/scripts/render-release-install.sh"
README_MD="$REPO_ROOT/README.md"
RELEASE_MD="$REPO_ROOT/docs/contributing/release.md"

pass=0
fail=0

_assert_contains() {
  local test_name="$1"
  local input="$2"
  local expected="$3"

  if grep -qF -- "$expected" <<<"$input"; then
    echo "  PASS: $test_name"
    ((pass++)) || true
  else
    echo "  FAIL: $test_name"
    echo "        expected to find: $expected"
    ((fail++)) || true
  fi
}

_assert_not_contains() {
  local test_name="$1"
  local input="$2"
  local unexpected="$3"

  if grep -qF -- "$unexpected" <<<"$input"; then
    echo "  FAIL: $test_name"
    echo "        did not expect to find: $unexpected"
    ((fail++)) || true
  else
    echo "  PASS: $test_name"
    ((pass++)) || true
  fi
}

echo "=== install.sh defaults ==="

script="$(cat "$INSTALL_SH")"
help_output="$(sh "$INSTALL_SH" --help)"

_assert_contains \
  "source installer defaults to latest release" \
  "$script" \
  'RELEASE_VERSION=latest'

_assert_not_contains \
  "usage does not expose version override" \
  "$help_output" \
  "--version"

_assert_not_contains \
  "usage does not document VERSION environment override" \
  "$help_output" \
  "VERSION"

_assert_contains \
  "default bin dir is user-local" \
  "$script" \
  'BIN_DIR=${TAXIWAY_INSTALL_BIN_DIR:-"$HOME/.local/bin"}'

_assert_contains \
  "usage documents bin dir environment override" \
  "$help_output" \
  "TAXIWAY_INSTALL_BIN_DIR"

_assert_contains \
  "usage documents user-local default" \
  "$help_output" \
  '$HOME/.local/bin'

_assert_contains \
  "runtime installs to unversioned user runtime" \
  "$script" \
  'runtime_dir="$HOME/.taxiway/runtime"'

_assert_contains \
  "runtime is staged before replacement" \
  "$script" \
  'runtime_tmp="$HOME/.taxiway/runtime.tmp"'

goreleaser_config="$(cat "$GORELEASER_YAML")"
readme="$(cat "$README_MD")"
release_doc="$(cat "$RELEASE_MD")"

_assert_contains \
  "goreleaser publishes install script as release asset" \
  "$goreleaser_config" \
  'release:'

_assert_contains \
  "goreleaser includes install script in release extra files" \
  "$goreleaser_config" \
  'glob: ./.release/install.sh'

_assert_contains \
  "goreleaser publishes generated script as install.sh" \
  "$goreleaser_config" \
  'name_template: install.sh'

_assert_contains \
  "goreleaser renders release installer with release version" \
  "$goreleaser_config" \
  'sh ./scripts/render-release-install.sh ./install.sh ./.release/install.sh {{ .Version }}'

tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT
sh "$RENDER_RELEASE_INSTALL_SH" "$INSTALL_SH" "$tmpdir/install.sh" "0.1.0"
rendered_installer="$(cat "$tmpdir/install.sh")"

_assert_contains \
  "rendered release installer pins release version" \
  "$rendered_installer" \
  'RELEASE_VERSION="0.1.0"'

_assert_not_contains \
  "rendered release installer does not keep latest default" \
  "$rendered_installer" \
  'RELEASE_VERSION=latest'

_assert_contains \
  "README installs from immutable release asset" \
  "$readme" \
  "https://github.com/taxiway-sh/taxiway/releases/latest/download/install.sh"

_assert_contains \
  "README installs explicit releases from release-specific asset" \
  "$readme" \
  "https://github.com/taxiway-sh/taxiway/releases/download/v0.1.0/install.sh"

_assert_not_contains \
  "README does not install explicit versions via latest installer" \
  "$readme" \
  "--version"

_assert_contains \
  "release guide installs from immutable release asset" \
  "$release_doc" \
  "https://github.com/taxiway-sh/taxiway/releases/latest/download/install.sh"

_assert_contains \
  "release guide installs explicit releases from release-specific asset" \
  "$release_doc" \
  "https://github.com/taxiway-sh/taxiway/releases/download/v0.1.0/install.sh"

_assert_not_contains \
  "release guide does not install explicit versions via latest installer" \
  "$release_doc" \
  "--version"

echo ""
echo "Results: $pass passed, $fail failed"
[[ $fail -eq 0 ]]
