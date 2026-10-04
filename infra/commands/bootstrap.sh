#!/usr/bin/env bash
# Install the system-level toolchains the lab depends on.
#
# This script is idempotent: re-running it should be cheap and safe. It only
# handles OS packages and language runtimes.

set -euo pipefail

log() { printf '\n\033[1;34m[bootstrap]\033[0m %s\n' "$*"; }
# shellcheck source=steps.sh
source "$(dirname "${BASH_SOURCE[0]}")/steps.sh"

export DEBIAN_FRONTEND=noninteractive

# Tolerate transient failures from third-party PPAs that may live on the host
# — apt-get still refreshes the sources it can reach. If the subsequent
# install step needs a stale index it will fail with a clear message.
taxiway_step "Updating apt cache" sudo apt-get update || log "apt-get update had errors (continuing)"

base_packages=(
  ca-certificates curl git make tmux asciinema jq unzip \
  bash-completion \
  build-essential pkg-config \
  ripgrep fd-find \
  lsof procps \
  python3 python3-pip python3-venv \
  openjdk-21-jdk-headless
)
log "Installing base packages"
taxiway_plan_detail "${base_packages[*]}"
taxiway_apply sudo apt-get install -y --no-install-recommends "${base_packages[@]}"

install_docker() {
  curl -fsSL https://get.docker.com | sh
  sudo usermod -aG docker "$USER"
}

if ! taxiway_can_inspect || ! command -v docker >/dev/null 2>&1; then
  taxiway_step "Installing Docker" install_docker
else
  log "Docker already installed ($(docker --version))"
fi

install_node() {
  curl -fsSL https://deb.nodesource.com/setup_22.x | sudo -E bash -
  sudo apt-get install -y nodejs
}

node_install_planned=false
if ! taxiway_can_inspect || ! command -v node >/dev/null 2>&1; then
  if taxiway_is_plan; then
    node_install_planned=true
  fi
  taxiway_step "Installing Node.js 22" install_node
else
  log "Node already installed ($(node --version))"
fi

# Enable corepack so pnpm/yarn are available when a workspace asks for them.
enable_corepack() {
  sudo corepack enable >/dev/null 2>&1
}
if [[ "$node_install_planned" == "true" ]] || command -v corepack >/dev/null 2>&1; then
  taxiway_step "Enabling Corepack" enable_corepack || true
fi

log "Toolchain summary"
if ! taxiway_can_inspect; then
  for tool in docker node npm python java git tmux asciinema; do
    printf '  %-10s : %s\n' "$tool" 'unknown (guest inspection unavailable)'
  done
else
  printf '  %-10s : %s\n' "docker" "$(docker --version 2>/dev/null || echo 'missing')"
  printf '  %-10s : %s\n' "node" "$(node --version 2>/dev/null || echo 'missing')"
  printf '  %-10s : %s\n' "npm" "$(npm --version 2>/dev/null || echo 'missing')"
  printf '  %-10s : %s\n' "python" "$(python3 --version 2>/dev/null || echo 'missing')"
  if command -v java >/dev/null 2>&1; then
    java_version="$(java -version 2>&1 | head -n1)"
  else
    java_version="missing"
  fi
  printf '  %-10s : %s\n' "java" "$java_version"
  printf '  %-10s : %s\n' "git" "$(git --version 2>/dev/null || echo 'missing')"
  printf '  %-10s : %s\n' "tmux" "$(tmux -V 2>/dev/null || echo 'missing')"
  printf '  %-10s : %s\n' "asciinema" "$(asciinema --version 2>/dev/null || echo 'missing')"
fi

# --- tmux configuration ---
TMUX_CONF="$HOME/.tmux.conf"
enable_tmux_mouse() {
  echo "set -g mouse on" >> "$TMUX_CONF"
}
if ! taxiway_can_inspect || ! grep -qF "set -g mouse on" "$TMUX_CONF" 2>/dev/null; then
    taxiway_step "Enabling tmux mouse support in $TMUX_CONF" enable_tmux_mouse
else
    log "tmux mouse support already configured, skipping"
fi


# --- taxiway env: source per-lab managed env if present ---
TAXIWAY_PROFILE_MARKER='# >>> taxiway-managed: do not edit between markers'
add_taxiway_profile_block() {
  cat >> "$HOME/.profile" << 'EOF'

# >>> taxiway-managed: do not edit between markers
if [ -f "$HOME/.config/taxiway/env" ]; then
    set -a
    . "$HOME/.config/taxiway/env"
    set +a
fi
# <<< taxiway-managed
EOF
}
if ! taxiway_can_inspect || ! grep -qF "$TAXIWAY_PROFILE_MARKER" "$HOME/.profile" 2>/dev/null; then
  taxiway_step "Configuring login shells to load the Taxiway environment" add_taxiway_profile_block
else
  log "Taxiway environment block already present in ~/.profile, skipping"
fi
