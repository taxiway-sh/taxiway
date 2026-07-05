#!/usr/bin/env bash
# Install the OpenAI `codex` CLI (npm package @openai/codex).

set -euo pipefail

source "$(dirname "${BASH_SOURCE[0]}")/../../infra/trace/events.sh" 2>/dev/null || true

lab_emit_event phase start

log() { printf '\n\033[1;34m[codex-agent-install]\033[0m %s\n' "$*"; }

VERSION="${CODEX_VERSION:-latest}"
PKG="@openai/codex"

command -v npm >/dev/null 2>&1 || { echo "npm missing - run taxiway bootstrap first" >&2; exit 1; }

if ! command -v bwrap >/dev/null 2>&1; then
  log "Installing bubblewrap (codex sandbox prerequisite)"
  sudo apt-get install -y --no-install-recommends bubblewrap
else
  log "bubblewrap already installed: $(bwrap --version 2>/dev/null || echo 'ok')"
fi

if command -v codex >/dev/null 2>&1; then
  current="$(codex --version 2>/dev/null | awk '{print $NF}' || true)"
  if [ "$VERSION" = "latest" ]; then
    log "codex already installed (version: ${current:-unknown}) - skipping"
    lab_emit_event phase done
    exit 0
  elif [ "$current" = "$VERSION" ]; then
    log "codex already installed (version: ${current:-unknown})"
    lab_emit_event phase done
    exit 0
  fi
fi

log "Installing ${PKG}@${VERSION}"
npm_err="$(mktemp -t npm-err.XXXXXX)"
if ! npm install -g "${PKG}@${VERSION}" 2>"$npm_err"; then
  if grep -qi 'EACCES\|permission denied' "$npm_err"; then
    log "Retrying with sudo"
    rm -f "$npm_err"
    sudo npm install -g "${PKG}@${VERSION}"
  else
    cat "$npm_err" >&2
    rm -f "$npm_err"
    exit 1
  fi
fi
rm -f "$npm_err"

CODEX_BIN="$(command -v codex || true)"
[ -x "$CODEX_BIN" ] || { echo "codex not found after install" >&2; exit 1; }

log "Installed: $CODEX_BIN ($("$CODEX_BIN" --version 2>/dev/null || echo '?'))"
lab_emit_event phase done
