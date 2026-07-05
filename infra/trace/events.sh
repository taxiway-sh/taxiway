#!/usr/bin/env bash
# infra/trace/events.sh — helper for emitting LAB_AGENT_EVENT lines
# from install/verify scripts.
#
# Usage:
#   source "$(dirname "$0")/../../infra/trace/events.sh" 2>/dev/null || true
#   lab_emit_event phase start
#   # ... work ...
#   lab_emit_event phase done
#
# Format: LAB_AGENT_EVENT {"type":"<type>","source":"<source>","ts":"<ts>","fields":{"phase":"<phase>"}}

lab_emit_event() {
  local type="${1:-phase}"
  local phase="${2:-}"
  local source="${LAB_SOURCE:-${ID:-unknown}}"
  local ts
  ts="$(date -u +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || echo "1970-01-01T00:00:00Z")"

  printf 'LAB_AGENT_EVENT {"type":"%s","source":"%s","ts":"%s","fields":{"phase":"%s"}}\n' \
    "$type" "$source" "$ts" "$phase"
}
