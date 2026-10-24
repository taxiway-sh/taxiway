#!/usr/bin/env bash
# tests/scripts/test_standalone_install.sh - contract tests for root install.sh defaults.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
INSTALL_SH="$SCRIPT_DIR/../../install.sh"
REPO_ROOT="$SCRIPT_DIR/../.."
GORELEASER_YAML="$REPO_ROOT/.goreleaser.yaml"
RENDER_RELEASE_INSTALL_SH="$REPO_ROOT/scripts/render-release-install.sh"

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

echo "=== foreign archive ownership ==="
python3 - "$INSTALL_SH" "$tmpdir/install.sh" <<'PYTHON'
import io
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tarfile
import tempfile

with tempfile.TemporaryDirectory(prefix="taxiway-install-ownership-") as root:
    root = Path(root)
    fixture = root / "fixture"
    fixture.mkdir()
    archive = fixture / "archive.tar.gz"
    payloads = {
        "taxiway": b"#!/bin/sh\nprintf 'fixture binary\\n'\n",
        "infra/fixture": b"infra\n",
        "agents/fixture": b"agents\n",
        "orchestrators/fixture": b"orchestrators\n",
    }
    foreign = 1001 if os.getuid() != 1001 else 1002
    with tarfile.open(archive, "w:gz") as stream:
        for name, payload in payloads.items():
            entry = tarfile.TarInfo(name)
            entry.size = len(payload)
            entry.mode = 0o755 if name == "taxiway" else 0o644
            entry.uid = entry.gid = foreign
            entry.uname = entry.gname = "foreign-builder"
            stream.addfile(entry, io.BytesIO(payload))
    bindir = root / "bin"
    bindir.mkdir()
    real_tar = shutil.which("tar")
    assert real_tar
    (bindir / "curl").write_text('''#!/usr/bin/env python3
import hashlib, json, os, pathlib, shutil, sys
args = sys.argv[1:]
url = next(arg for arg in args if arg.startswith("https://"))
if url.endswith("/latest"):
    print(json.dumps({"tag_name": "v0.1.0"}))
else:
    output = pathlib.Path(args[args.index("-o") + 1])
    archive = pathlib.Path(os.environ["INSTALL_OWNERSHIP_FIXTURE"]) / "archive.tar.gz"
    if url.endswith("/checksums.txt"):
        name = os.environ["INSTALL_OWNERSHIP_ARCHIVE_NAME"]
        output.write_text(hashlib.sha256(archive.read_bytes()).hexdigest() + "  " + name + "\\n")
    else:
        shutil.copyfile(archive, output)
''')
    # Observe actual extraction ownership before the installer copies assets.
    # Checking only the installed copies would miss tar restoring builder IDs.
    (bindir / "tar").write_text('''#!/usr/bin/env python3
import os, pathlib, subprocess, sys
args = sys.argv[1:]
subprocess.run([os.environ["INSTALL_OWNERSHIP_REAL_TAR"], *args], check=True)
destination = pathlib.Path(args[args.index("-C") + 1])
for path in destination.rglob("*"):
    if path.name.endswith((".tar.gz", ".txt")):
        continue
    info = path.stat()
    assert (info.st_uid, info.st_gid) == (os.getuid(), os.getgid()), path.name
''')
    for path in bindir.iterdir():
        path.chmod(0o755)
    system = {"Darwin": "darwin", "Linux": "linux"}[os.uname().sysname]
    arch = {"x86_64": "amd64", "amd64": "amd64", "arm64": "arm64", "aarch64": "arm64"}[os.uname().machine]
    for index, installer in enumerate(sys.argv[1:]):
        home = root / str(index)
        home.mkdir()
        env = dict(os.environ, HOME=str(home), PATH=str(bindir) + os.pathsep + os.environ["PATH"],
                   TAXIWAY_INSTALL_BIN_DIR=str(home / "bin"),
                   INSTALL_OWNERSHIP_FIXTURE=str(fixture),
                   INSTALL_OWNERSHIP_REAL_TAR=real_tar,
                   INSTALL_OWNERSHIP_ARCHIVE_NAME=f"taxiway_0.1.0_{system}_{arch}.tar.gz")
        result = subprocess.run(["sh", installer], env=env, capture_output=True, text=True)
        assert result.returncode == 0, result.stdout + result.stderr
        assert (home / "bin/taxiway").read_bytes() == payloads["taxiway"]
        assert os.access(home / "bin/taxiway", os.X_OK)
        for name in ("infra", "agents", "orchestrators"):
            path = home / ".taxiway/runtime" / name / "fixture"
            assert path.read_bytes() == payloads[name + "/fixture"]
            assert path.stat().st_uid == os.getuid()
        print("  PASS: foreign-owned archive installs with caller ownership (" +
              ("source" if index == 0 else "release") + " installer)")
PYTHON
((pass += 2))

echo ""
echo "Results: $pass passed, $fail failed"
[[ $fail -eq 0 ]]
