#!/bin/sh
set -eu

OWNER=${TAXIWAY_INSTALL_OWNER:-taxiway-sh}
REPO=${TAXIWAY_INSTALL_REPO:-taxiway}
BIN_NAME=taxiway
BIN_DIR=${TAXIWAY_INSTALL_BIN_DIR:-"$HOME/.local/bin"}
RELEASE_VERSION=latest

usage() {
  cat <<'USAGE'
Install the taxiway CLI from GitHub Releases.

Usage:
  install.sh [--bin-dir DIR]

Environment:
  TAXIWAY_INSTALL_OWNER    GitHub owner override for development.
  TAXIWAY_INSTALL_REPO     GitHub repository override for development.
  TAXIWAY_INSTALL_BIN_DIR  Binary directory. Defaults to $HOME/.local/bin.
USAGE
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --bin-dir)
      [ "$#" -ge 2 ] || { echo "error: --bin-dir requires a value" >&2; exit 1; }
      BIN_DIR=$2
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "error: unknown argument $1" >&2
      usage >&2
      exit 1
      ;;
  esac
done

need() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "error: missing required command: $1" >&2
    exit 1
  fi
}

need curl
need awk
need cp
need grep
need install
need tar
need sed
need uname
need mktemp

case "$(uname -s)" in
  Darwin) os=darwin ;;
  Linux) os=linux ;;
  *) echo "error: unsupported OS: $(uname -s)" >&2; exit 1 ;;
esac

case "$(uname -m)" in
  x86_64|amd64) arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *) echo "error: unsupported architecture: $(uname -m)" >&2; exit 1 ;;
esac

selected_release=$RELEASE_VERSION
if [ "$selected_release" = "latest" ]; then
  latest_url="https://api.github.com/repos/$OWNER/$REPO/releases/latest"
  selected_release=$(curl -fsSL "$latest_url" | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)
  if [ -z "$selected_release" ]; then
    echo "error: could not resolve latest release for $OWNER/$REPO" >&2
    exit 1
  fi
fi

release_version=${selected_release#v}
tag="v$release_version"
archive="${BIN_NAME}_${release_version}_${os}_${arch}.tar.gz"
base_url="https://github.com/$OWNER/$REPO/releases/download/$tag"
tmpdir=$(mktemp -d)

cleanup() {
  rm -rf "$tmpdir"
}
trap cleanup EXIT INT TERM

echo "Downloading $archive"
curl -fsSL "$base_url/$archive" -o "$tmpdir/$archive"
curl -fsSL "$base_url/checksums.txt" -o "$tmpdir/checksums.txt"

checksum_line=$(grep " $archive\$" "$tmpdir/checksums.txt" || true)
if [ -z "$checksum_line" ]; then
  echo "error: checksum for $archive not found in checksums.txt" >&2
  exit 1
fi

expected=$(printf '%s\n' "$checksum_line" | awk '{print $1}')
if command -v sha256sum >/dev/null 2>&1; then
  actual=$(sha256sum "$tmpdir/$archive" | awk '{print $1}')
elif command -v shasum >/dev/null 2>&1; then
  actual=$(shasum -a 256 "$tmpdir/$archive" | awk '{print $1}')
else
  echo "error: missing sha256sum or shasum for checksum verification" >&2
  exit 1
fi

if [ "$actual" != "$expected" ]; then
  echo "error: checksum verification failed for $archive" >&2
  exit 1
fi

tar -xzf "$tmpdir/$archive" -C "$tmpdir"

if [ ! -f "$tmpdir/$BIN_NAME" ]; then
  echo "error: release archive does not contain $BIN_NAME" >&2
  exit 1
fi

mkdir -p "$BIN_DIR"

if [ ! -w "$BIN_DIR" ]; then
  echo "error: binary directory is not writable: $BIN_DIR" >&2
  echo "hint: rerun with a writable --bin-dir, or install manually with elevated privileges" >&2
  exit 1
fi

runtime_dir="$HOME/.taxiway/runtime"
runtime_tmp="$HOME/.taxiway/runtime.tmp"
mkdir -p "$HOME/.taxiway"
rm -rf "$runtime_tmp"
mkdir -p "$runtime_tmp"

for path in infra agents orchestrators; do
  if [ -e "$tmpdir/$path" ]; then
    cp -R "$tmpdir/$path" "$runtime_tmp/"
  fi
done

rm -rf "$runtime_dir"
mv "$runtime_tmp" "$runtime_dir"
install -m 0755 "$tmpdir/$BIN_NAME" "$BIN_DIR/$BIN_NAME"

echo "Installed $BIN_NAME $release_version to $BIN_DIR/$BIN_NAME"
echo "Installed taxiway runtime assets to $runtime_dir"
case ":$PATH:" in
  *":$BIN_DIR:"*) ;;
  *) echo "Add $BIN_DIR to PATH before running '$BIN_NAME' without its full path." ;;
esac
echo "Run 'taxiway init' to initialize the Taxiway runtime."
