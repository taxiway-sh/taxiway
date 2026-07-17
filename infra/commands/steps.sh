#!/usr/bin/env bash

taxiway_is_plan() {
  [[ "${TAXIWAY_EXECUTION_MODE:-apply}" == "plan" ]]
}

taxiway_can_inspect() {
  [[ "${TAXIWAY_PLAN_INSPECTION:-available}" == "available" ]]
}

taxiway_plan_step() {
  taxiway_is_plan || return 1
  log "$1"
  return 0
}

taxiway_step() {
  local label="$1"
  shift
  log "$label"
  if taxiway_is_plan; then
    return 0
  fi
  "$@"
}

taxiway_apply() {
  if taxiway_is_plan; then
    return 0
  fi
  "$@"
}

taxiway_plan_detail() {
  if taxiway_is_plan; then
    printf '  %s\n' "$*"
  fi
}

taxiway_plan_detail_if_unavailable() {
  if taxiway_is_plan && ! taxiway_can_inspect; then
    printf '  %s\n' "$*"
  fi
}
