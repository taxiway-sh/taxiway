#!/usr/bin/env bash
# Install the Gas City CLI (`gc`) from https://github.com/gastownhall/gascity.
#
# Idempotent. Installs exact releases of gc, Beads (`bd`) and Dolt to
# $HOME/.local/bin. Gas City and Beads archives are verified against their
# published checksums; the default Dolt archive is verified against a pinned
# digest. Also installs the process tools Gas City relies on (lsof, pgrep,
# flock, jq).

set -euo pipefail

# shellcheck source=../../infra/trace/events.sh
source "$(dirname "${BASH_SOURCE[0]}")/../../infra/trace/events.sh" 2>/dev/null || true
# shellcheck source=../../infra/commands/steps.sh
source "$(dirname "${BASH_SOURCE[0]}")/../../infra/commands/steps.sh"

log() { printf '\n\033[1;34m[gascity-install]\033[0m %s\n' "$*"; }
die() { printf 'ERROR: %s\n' "$*" >&2; exit 1; }

export PATH="$HOME/.local/bin:$PATH"

GASCITY_VERSION="${TAXIWAY_SET_VERSION:-1.5.0}"
SEMVER='^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$'

# Releases are exact so a lab is reproducible: Gas City has no "latest" mode.
resolve_versions() {
  GASCITY_VERSION="${GASCITY_VERSION#v}"
  [[ "$GASCITY_VERSION" =~ $SEMVER ]] || die "Gas City version must be an exact release such as 1.5.0 (got '${GASCITY_VERSION}')"

  # Gas City's own CI pins (deps.env) for each supported release.
  local mapped_beads="" mapped_dolt=""
  case "$GASCITY_VERSION" in
    1.5.0) mapped_beads=1.3.1 mapped_dolt=2.2.0 ;;
  esac

  BEADS_VERSION="${TAXIWAY_SET_BEADS_VERSION:-$mapped_beads}"
  DOLT_VERSION="${TAXIWAY_SET_DOLT_VERSION:-$mapped_dolt}"
  BEADS_VERSION="${BEADS_VERSION#v}"
  DOLT_VERSION="${DOLT_VERSION#v}"
  [[ -n "$BEADS_VERSION" ]] || die "no Beads compatibility mapping for Gas City ${GASCITY_VERSION}; set --set beads-version=<version>"
  [[ -n "$DOLT_VERSION" ]] || die "no Dolt compatibility mapping for Gas City ${GASCITY_VERSION}; set --set dolt-version=<version>"
  [[ "$BEADS_VERSION" =~ $SEMVER ]] || die "Beads version must be an exact release (got '${BEADS_VERSION}')"
  [[ "$DOLT_VERSION" =~ $SEMVER ]] || die "Dolt version must be an exact release (got '${DOLT_VERSION}')"
}

installed_version() {
  local binary="$1"
  command -v "$binary" >/dev/null 2>&1 || return 0
  # Read the whole output: exiting early sends SIGPIPE to a binary that keeps
  # writing (Dolt prints an upgrade notice after its version), and pipefail
  # then fails the phase.
  "$binary" version 2>/dev/null | awk '
    !found {
      for (i = 1; i <= NF; i++) {
        if ($i ~ /^v?[0-9]+\.[0-9]+\.[0-9]+([-+][0-9A-Za-z.-]+)?$/) {
          sub(/^v/, "", $i)
          print $i
          found = 1
          break
        }
      }
    }'
}

resolve_versions

if taxiway_is_plan; then
  if taxiway_can_inspect && command -v lsof >/dev/null 2>&1 && command -v pgrep >/dev/null 2>&1 \
    && command -v flock >/dev/null 2>&1 && command -v jq >/dev/null 2>&1; then
    log "apt dependencies already installed"
  else
    log "Installing apt dependencies"
    taxiway_plan_detail "lsof procps util-linux jq"
  fi
  for spec in "dolt:Dolt:$DOLT_VERSION" "bd:Beads:$BEADS_VERSION" "gc:Gas City:$GASCITY_VERSION"; do
    IFS=: read -r binary label version <<<"$spec"
    if taxiway_can_inspect && [[ "$(installed_version "$binary")" == "$version" ]]; then
      log "$label $version already installed"
    else
      log "Installing $label $version"
    fi
  done
  log "Configuring Dolt identity and Beads role when missing"
  log "Disabling Beads anonymous usage metrics"
  exit 0
fi

lab_emit_event phase start

download() {
  curl --fail --silent --show-error --location \
    --retry 5 --retry-all-errors --retry-delay 3 --connect-timeout 20 \
    "$1" -o "$2"
}

release_arch() {
  case "$(uname -m)" in
    x86_64) echo amd64 ;;
    aarch64|arm64) echo arm64 ;;
    *) die "unsupported architecture $(uname -m)" ;;
  esac
}

# verify_sha256 <file> <expected-digest>
verify_sha256() {
  printf '%s  %s\n' "$2" "$1" | sha256sum --check --status || die "checksum mismatch for $(basename "$1")"
}

# verify_published_checksum <file> <asset-name> <checksums-url>
verify_published_checksum() {
  local file="$1" asset="$2" url="$3" sums expected
  sums="$(dirname "$file")/checksums.txt"
  download "$url" "$sums"
  expected="$(awk -v asset="$asset" '$2 == asset || $2 == "*" asset { print $1; exit }' "$sums")"
  [[ -n "$expected" ]] || die "$asset is missing from $url"
  verify_sha256 "$file" "$expected"
}

ARCH="$(release_arch)"

# Downloads go to one work directory, removed on every exit path.
WORK_DIR="$(mktemp -d)"
trap 'rm -rf "$WORK_DIR"' EXIT

install_apt_deps() {
  if command -v lsof >/dev/null 2>&1 && command -v pgrep >/dev/null 2>&1 \
    && command -v flock >/dev/null 2>&1 && command -v jq >/dev/null 2>&1; then
    log "apt dependencies already installed"
    return 0
  fi
  log "Installing apt dependencies"
  sudo apt-get update || true
  sudo DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends lsof procps util-linux jq
}

install_dolt() {
  if [[ "$(installed_version dolt)" == "$DOLT_VERSION" ]]; then
    log "Dolt $DOLT_VERSION already installed"
    return 0
  fi
  log "Installing Dolt $DOLT_VERSION"
  local tmp archive expected=""
  tmp="$(mktemp -d "$WORK_DIR/dolt.XXXXXX")"
  archive="$tmp/dolt.tar.gz"
  download "https://github.com/dolthub/dolt/releases/download/v${DOLT_VERSION}/dolt-linux-${ARCH}.tar.gz" "$archive"
  # Dolt publishes no checksum file; the default release digest is pinned here.
  case "${DOLT_VERSION}/${ARCH}" in
    2.2.0/amd64) expected=1f7ad8c2622995789420a3fb0f2d16b4aa7430000a825dd91d5938f36480cbf6 ;;
    2.2.0/arm64) expected=a49a566d7c1ee9fdff553644855737f9d87047557c1ce1951aa7e7053277f4af ;;
  esac
  if [[ -n "$expected" ]]; then
    verify_sha256 "$archive" "$expected"
  else
    echo "WARN: no pinned digest for Dolt ${DOLT_VERSION}/${ARCH}; archive integrity relies on HTTPS" >&2
  fi
  tar -C "$tmp" -xzf "$archive"
  mkdir -p "$HOME/.local/bin"
  install -m 0755 "$tmp/dolt-linux-${ARCH}/bin/dolt" "$HOME/.local/bin/dolt"
  rm -rf "$tmp"
}

# install_goreleaser_binary <repo> <asset-prefix> <version> <binary> <checksums-file> <label>
install_goreleaser_binary() {
  local repo="$1" prefix="$2" version="$3" binary="$4" sums_name="$5" label="$6"
  if [[ "$(installed_version "$binary")" == "$version" ]]; then
    log "$label $version already installed"
    return 0
  fi
  log "Installing $label $version"
  local tmp asset base
  tmp="$(mktemp -d "$WORK_DIR/$binary.XXXXXX")"
  asset="${prefix}_${version}_linux_${ARCH}.tar.gz"
  base="https://github.com/gastownhall/${repo}/releases/download/v${version}"
  download "$base/$asset" "$tmp/$asset"
  verify_published_checksum "$tmp/$asset" "$asset" "$base/$sums_name"
  tar -C "$tmp" -xzf "$tmp/$asset" "$binary"
  mkdir -p "$HOME/.local/bin"
  install -m 0755 "$tmp/$binary" "$HOME/.local/bin/$binary"
  rm -rf "$tmp"
}

configure_dolt_identity() {
  dolt config --global --get user.name >/dev/null 2>&1 \
    || dolt config --global --add user.name "Agent Lab"
  dolt config --global --get user.email >/dev/null 2>&1 \
    || dolt config --global --add user.email "agent-lab@example.local"
}

# Beads refuses some writes without a role; Gas City's doctor requires it too.
configure_beads_role() {
  git config --global beads.role >/dev/null 2>&1 \
    || git config --global beads.role maintainer
}

install_apt_deps
install_dolt
configure_dolt_identity
configure_beads_role
install_goreleaser_binary beads beads "$BEADS_VERSION" bd checksums.txt Beads
install_goreleaser_binary gascity gascity "$GASCITY_VERSION" gc "gascity_${GASCITY_VERSION}_checksums.txt" "Gas City"

# Beads enables anonymous usage metrics by default; labs opt out (idempotent,
# stored in ~/.config/bd).
log "Disabling Beads anonymous usage metrics"
bd metrics off >/dev/null

for binary in gc bd dolt; do
  actual="$(installed_version "$binary")"
  [[ -n "$actual" ]] || die "$binary binary not found after install"
done
[[ "$(installed_version gc)" == "$GASCITY_VERSION" ]] || die "gc reports $(installed_version gc), expected $GASCITY_VERSION"

log "Installed tools"
printf '  gc   : %s\n' "$(installed_version gc)"
printf '  bd   : %s\n' "$(installed_version bd)"
printf '  dolt : %s\n' "$(installed_version dolt)"

log "Done"

lab_emit_event phase done
