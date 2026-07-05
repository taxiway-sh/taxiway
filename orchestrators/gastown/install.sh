#!/usr/bin/env bash
# Install the `gt` binary from https://github.com/gastownhall/gastown.
#
# Idempotent. Installs the Gas Town CLI to $HOME/.local/bin from upstream
# release archives. Also installs runtime dependencies required by upstream
# docs/build: Dolt, Beads (`bd`), sqlite3, and ICU headers.

set -euo pipefail

# shellcheck source=../../infra/trace/events.sh
source "$(dirname "${BASH_SOURCE[0]}")/../../infra/trace/events.sh" 2>/dev/null || true

lab_emit_event phase start

log() { printf '\n\033[1;34m[gastown-install]\033[0m %s\n' "$*"; }

download() {
  local url="$1"
  local out="$2"
  curl --fail --silent --show-error --location \
    --retry 5 --retry-all-errors --retry-delay 3 --connect-timeout 20 \
    "$url" -o "$out"
}

install_dolt() {
  if command -v dolt >/dev/null 2>&1; then
    log "Dolt already installed ($(dolt version))"
    return 0
  fi

  log "Installing Dolt"
  download "https://github.com/dolthub/dolt/releases/latest/download/install.sh" /tmp/dolt-install.sh
  sudo bash /tmp/dolt-install.sh
  rm -f /tmp/dolt-install.sh
}

install_apt_deps() {
  if command -v sqlite3 >/dev/null 2>&1 && [ -f /usr/include/unicode/uregex.h ]; then
    log "apt dependencies already installed"
    return 0
  fi

  log "Installing apt dependencies"
  sudo apt-get update || true
  sudo apt-get install -y --no-install-recommends sqlite3 libicu-dev
}

resolve_gastown_install_ref() {
  GASTOWN_VERSION="${TAXIWAY_SET_VERSION:-latest}"
  GASTOWN_INSTALL_REF="$GASTOWN_VERSION"
  if [[ "$GASTOWN_VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]]; then
    GASTOWN_INSTALL_REF="v${GASTOWN_VERSION}"
  fi
}

resolve_latest_release_ref() {
  local owner="$1"
  local repo="$2"
  local url release_ref
  url="$(curl --fail --silent --show-error --location \
    --retry 5 --retry-all-errors --retry-delay 3 --connect-timeout 20 \
    --output /dev/null --write-out '%{url_effective}' \
    "https://github.com/${owner}/${repo}/releases/latest")" || return 1
  release_ref="${url##*/}"
  [[ "$release_ref" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]] || return 1
  printf '%s\n' "$release_ref"
}

github_release_arch() {
  case "$(uname -m)" in
    x86_64) echo amd64 ;;
    aarch64|arm64) echo arm64 ;;
    *) return 1 ;;
  esac
}

installed_tool_version() {
  local binary="$1"
  local out
  out="$("$binary" version 2>/dev/null || "$binary" --version 2>/dev/null || true)"
  printf '%s\n' "$out" | awk '
    {
      for (i = 1; i <= NF; i++) {
        if ($i ~ /^v?[0-9]+\.[0-9]+\.[0-9]+([-+][0-9A-Za-z.-]+)?$/) {
          sub(/^v/, "", $i)
          print $i
          exit
        }
      }
    }
  '
}

install_github_release_binary() {
  local owner="$1"
  local repo="$2"
  local asset_prefix="$3"
  local ref="$4"
  local binary="$5"
  local release_ref="$ref"

  if [ "$release_ref" = "latest" ]; then
    if command -v "$binary" >/dev/null 2>&1; then
      local current
      current="$(installed_tool_version "$binary" || true)"
      log "$binary already installed (version: ${current:-unknown}) - skipping"
      return 0
    fi

    release_ref="$(resolve_latest_release_ref "$owner" "$repo")"
    if [ -z "$release_ref" ]; then
      log "Release archive unavailable for ${owner}/${repo} ${ref}"
      return 1
    fi
  fi

  if [[ ! "$release_ref" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]]; then
    log "Release archive unavailable for ${owner}/${repo} ${ref}"
    return 1
  fi

  local arch
  if ! arch="$(github_release_arch)"; then
    log "Release archive unavailable for ${owner}/${repo} ${ref}"
    return 1
  fi

  local version="${release_ref#v}"
  if command -v "$binary" >/dev/null 2>&1; then
    local current
    current="$(installed_tool_version "$binary" || true)"
    if [ "$current" = "$version" ]; then
      log "$binary already installed (version: ${current:-unknown}) - skipping"
      return 0
    fi
  fi

  local archive_name="${asset_prefix}_${version}_linux_${arch}.tar.gz"
  local url="https://github.com/${owner}/${repo}/releases/download/${release_ref}/${archive_name}"
  local tmpdir archive bin target
  tmpdir="$(mktemp -d)"
  archive="$tmpdir/$archive_name"

  if ! download "$url" "$archive"; then
    rm -rf "$tmpdir"
    log "Release archive unavailable for ${owner}/${repo} ${ref}"
    return 1
  fi

  if ! tar -C "$tmpdir" -xzf "$archive"; then
    rm -rf "$tmpdir"
    log "Release archive unavailable for ${owner}/${repo} ${ref}"
    return 1
  fi

  bin="$tmpdir/$binary"
  if [ ! -x "$bin" ]; then
    rm -rf "$tmpdir"
    log "Release archive unavailable for ${owner}/${repo} ${ref}"
    return 1
  fi

  mkdir -p "$HOME/.local/bin"
  target="$HOME/.local/bin/$binary"
  install -m 0755 "$bin" "$target"
  rm -rf "$tmpdir"
}

resolve_beads_install_ref() {
  BEADS_VERSION="${TAXIWAY_SET_BEADS_VERSION:-}"
  if [[ -n "$BEADS_VERSION" ]]; then
    BEADS_INSTALL_REF="$BEADS_VERSION"
    if [[ "$BEADS_VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]]; then
      BEADS_INSTALL_REF="v${BEADS_VERSION}"
    fi
    return
  fi

  case "$GASTOWN_INSTALL_REF" in
    "1.0.0"|"v1.0.0") BEADS_INSTALL_REF="v1.0.0" ;;
    "1.0.1"|"v1.0.1") BEADS_INSTALL_REF="v1.0.0" ;;
    "1.1.0"|"v1.1.0") BEADS_INSTALL_REF="v1.0.4" ;;
    "1.2.1"|"v1.2.1"|latest) BEADS_INSTALL_REF="v1.0.4" ;;
    *) echo "no Beads compatibility mapping for Gas Town ${GASTOWN_INSTALL_REF}; set --set beads-version=<version>" >&2; exit 1 ;;
  esac
}

install_gastown() {
  log "Installing Gas Town"
  install_github_release_binary gastownhall gastown gastown "$GASTOWN_INSTALL_REF" gt
}

install_beads() {
  resolve_beads_install_ref
  log "Installing Beads"
  install_github_release_binary gastownhall beads beads "$BEADS_INSTALL_REF" bd
}

configure_dolt_identity() {
  local needs_name=0
  local needs_email=0
  dolt config --global --get user.name >/dev/null 2>&1 || needs_name=1
  dolt config --global --get user.email >/dev/null 2>&1 || needs_email=1

  if [ "$needs_name" -eq 0 ] && [ "$needs_email" -eq 0 ]; then
    return 0
  fi

  log "Configuring Dolt identity"
  if [ "$needs_name" -eq 1 ]; then
    dolt config --global --add user.name "Agent Lab"
  fi

  if [ "$needs_email" -eq 1 ]; then
    dolt config --global --add user.email "agent-lab@example.local"
  fi
}

export PATH="$HOME/.local/bin:$PATH"

resolve_gastown_install_ref
install_dolt
configure_dolt_identity
install_apt_deps

install_gastown
install_beads

GT_BIN="$(command -v gt || echo "$HOME/.local/bin/gt")"
BD_BIN="$(command -v bd || echo "$HOME/.local/bin/bd")"

[ -x "$GT_BIN" ] || { echo "gt binary not found after install" >&2; exit 1; }
[ -x "$BD_BIN" ] || { echo "bd binary not found after install" >&2; exit 1; }

log "Installed tools"
printf '  gt   : %s\n' "$("$GT_BIN" version 2>/dev/null || "$GT_BIN" --version 2>/dev/null || echo "$GT_BIN")"
printf '  bd   : %s\n' "$("$BD_BIN" version 2>/dev/null || echo "$BD_BIN")"
printf '  dolt : %s\n' "$(dolt version 2>/dev/null || echo 'missing')"
printf '  sqlite3 : %s\n' "$(sqlite3 --version 2>/dev/null | awk '{print $1}' || echo 'missing')"
printf '  icu headers : %s\n' "$([ -f /usr/include/unicode/uregex.h ] && echo present || echo missing)"

install_completions() {
  # bash — sourced automatically on Ubuntu via /etc/bash_completion.d/
  if command -v bash >/dev/null 2>&1 && [ -d /etc/bash_completion.d ]; then
    log "Installing gt bash completion"
    "$GT_BIN" completion bash | sudo tee /etc/bash_completion.d/gt >/dev/null
  fi

  # zsh — written to the standard vendor-completions directory in $fpath (Debian/Ubuntu)
  if command -v zsh >/dev/null 2>&1; then
    local zsh_site_functions="/usr/share/zsh/vendor-completions"
    sudo mkdir -p "$zsh_site_functions"
    log "Installing gt zsh completion"
    "$GT_BIN" completion zsh | sudo tee "${zsh_site_functions}/_gt" >/dev/null
  fi
}

install_completions

log "Done"

lab_emit_event phase done
