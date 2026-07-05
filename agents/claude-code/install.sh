#!/usr/bin/env bash
# Install the Anthropic `claude` CLI (npm package @anthropic-ai/claude-code).

set -euo pipefail

source "$(dirname "${BASH_SOURCE[0]}")/../../infra/trace/events.sh" 2>/dev/null || true

lab_emit_event phase start

log() { printf '\n\033[1;34m[claude-code-agent-install]\033[0m %s\n' "$*"; }

VERSION="${CLAUDE_CODE_VERSION:-latest}"
PKG="@anthropic-ai/claude-code"

command -v npm >/dev/null 2>&1 || { echo "npm missing - run taxiway bootstrap first" >&2; exit 1; }

if command -v claude >/dev/null 2>&1; then
  current="$(claude --version 2>/dev/null | awk '{print $1}' || true)"
  if [ "$VERSION" = "latest" ]; then
    log "claude already installed (version: ${current:-unknown}) - skipping"
    lab_emit_event phase done
    exit 0
  elif [ "$current" = "$VERSION" ]; then
    log "claude already installed (version: ${current:-unknown})"
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

CLAUDE_BIN="$(command -v claude || true)"
[ -x "$CLAUDE_BIN" ] || { echo "claude not found after install" >&2; exit 1; }

log "Installed: $CLAUDE_BIN ($("$CLAUDE_BIN" --version 2>/dev/null || echo '?'))"
lab_emit_event phase done
