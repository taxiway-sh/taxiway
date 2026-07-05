#!/usr/bin/env bash
# Install the system-level toolchains the lab depends on.
#
# This script is idempotent: re-running it should be cheap and safe. It only
# handles OS packages and language runtimes.

set -euo pipefail

log() { printf '\n\033[1;34m[bootstrap]\033[0m %s\n' "$*"; }

export DEBIAN_FRONTEND=noninteractive

log "Updating apt cache"
# Tolerate transient failures from third-party PPAs that may live on the host
# — apt-get still refreshes the sources it can reach. If the subsequent
# install step needs a stale index it will fail with a clear message.
sudo apt-get update || log "apt-get update had errors (continuing)"

log "Installing base packages"
sudo apt-get install -y --no-install-recommends \
  ca-certificates curl git make tmux asciinema jq unzip \
  bash-completion \
  build-essential pkg-config \
  ripgrep fd-find \
  lsof procps \
  python3 python3-pip python3-venv \
  openjdk-21-jdk-headless

if ! command -v docker >/dev/null 2>&1; then
  log "Installing Docker"
  curl -fsSL https://get.docker.com | sh
  sudo usermod -aG docker "$USER"
else
  log "Docker already installed ($(docker --version))"
fi

if ! command -v node >/dev/null 2>&1; then
  log "Installing Node.js 22"
  curl -fsSL https://deb.nodesource.com/setup_22.x | sudo -E bash -
  sudo apt-get install -y nodejs
else
  log "Node already installed ($(node --version))"
fi

# Enable corepack so pnpm/yarn are available when a workspace asks for them.
if command -v corepack >/dev/null 2>&1; then
  sudo corepack enable >/dev/null 2>&1 || true
fi

log "Toolchain summary"
printf '  %-10s : %s\n' "docker" "$(docker --version 2>/dev/null || echo 'missing')"
printf '  %-10s : %s\n' "node" "$(node --version 2>/dev/null || echo 'missing')"
printf '  %-10s : %s\n' "npm" "$(npm --version 2>/dev/null || echo 'missing')"
printf '  %-10s : %s\n' "python" "$(python3 --version 2>/dev/null || echo 'missing')"
printf '  %-10s : %s\n' "java" "$(java -version 2>&1 | head -n1 || echo 'missing')"
printf '  %-10s : %s\n' "git" "$(git --version 2>/dev/null || echo 'missing')"
printf '  %-10s : %s\n' "tmux" "$(tmux -V 2>/dev/null || echo 'missing')"
printf '  %-10s : %s\n' "asciinema" "$(asciinema --version 2>/dev/null || echo 'missing')"

log "Done. If docker was just installed, reconnect the shell to pick up the docker group."

# --- tmux configuration ---
TMUX_CONF="$HOME/.tmux.conf"
if ! grep -qF "set -g mouse on" "$TMUX_CONF" 2>/dev/null; then
    echo "set -g mouse on" >> "$TMUX_CONF"
    echo "[bootstrap] tmux mouse support enabled in $TMUX_CONF"
else
    echo "[bootstrap] tmux mouse support already configured, skipping"
fi


# --- taxiway env: source per-lab managed env if present ---
TAXIWAY_PROFILE_MARKER='# >>> taxiway-managed: do not edit between markers'
if ! grep -qF "$TAXIWAY_PROFILE_MARKER" "$HOME/.profile" 2>/dev/null; then
  cat >> "$HOME/.profile" << 'EOF'

# >>> taxiway-managed: do not edit between markers
if [ -f "$HOME/.config/taxiway/env" ]; then
    set -a
    . "$HOME/.config/taxiway/env"
    set +a
fi
# <<< taxiway-managed
EOF
  echo "[bootstrap] taxiway env block added to ~/.profile"
else
  echo "[bootstrap] taxiway env block already present in ~/.profile, skipping"
fi
