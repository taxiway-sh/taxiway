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

# The driver tells phase scripts which environment type the lab provides.
log "Guest environment: ${TAXIWAY_GUEST_KIND:-unknown}"

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
  # A successful setup script is insufficient: key downloads have failed while
  # the script returned success, leaving apt to select Ubuntu's Node.js package.
  local source configured=false candidate
  if sudo test -s /usr/share/keyrings/nodesource.gpg; then
    for source in /etc/apt/sources.list.d/nodesource.sources /etc/apt/sources.list.d/nodesource.list; do
      if sudo grep -Fq 'https://deb.nodesource.com/node_22.x' "$source" 2>/dev/null &&
          sudo grep -Fq '/usr/share/keyrings/nodesource.gpg' "$source" 2>/dev/null; then
        configured=true
        break
      fi
    done
  fi
  if [[ "$configured" != true ]]; then
    log "NodeSource setup did not configure its Node.js 22 repository and signing key; rerun bootstrap after fixing repository access."
    return 1
  fi
  candidate="$(LC_ALL=C apt-cache madison nodejs | awk '$3 ~ /^([0-9]+:)?22\./ && /https:\/\/deb\.nodesource\.com\/node_22\.x/ && !found {print $3; found=1}')"
  if [[ -z "$candidate" ]]; then
    log "NodeSource did not provide a Node.js 22 installation candidate; rerun bootstrap after fixing repository access."
    return 1
  fi
  # Select the repository's version explicitly, including when a different
  # major is already installed and apt would otherwise keep the newer package.
  sudo apt-get install -y --allow-downgrades "nodejs=$candidate"
  if ! node_toolchain_ready; then
    log "Node.js 22 and a working npm are required; the installation is incomplete. Rerun bootstrap to repair it."
    return 1
  fi
}

node_toolchain_ready() {
  local version
  command -v node >/dev/null 2>&1 && command -v npm >/dev/null 2>&1 || return 1
  version="$(node --version)" || return 1
  [[ "$version" =~ ^v22\. ]] && npm --version >/dev/null 2>&1
}

node_install_planned=false
if ! taxiway_can_inspect || ! node_toolchain_ready; then
  if taxiway_is_plan; then
    node_install_planned=true
  fi
  taxiway_step "Installing Node.js 22" install_node
else
  log "Node already installed ($(node --version))"
fi

if ! taxiway_is_plan; then
  for tool in docker npm python3 java git tmux asciinema; do
    if ! command -v "$tool" >/dev/null 2>&1; then
      log "Required tool $tool is missing; rerun bootstrap to repair the toolchain."
      exit 1
    fi
    version_flag=--version
    case "$tool" in java) version_flag=-version ;; tmux) version_flag=-V ;; esac
    if ! "$tool" "$version_flag" >/dev/null 2>&1; then
      log "Required tool $tool is not usable; rerun bootstrap to repair the toolchain."
      exit 1
    fi
  done
  if ! node_toolchain_ready; then
    log "Node.js 22 and a working npm are required; rerun bootstrap to repair the toolchain."
    exit 1
  fi
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
