#!/usr/bin/env bash

taxiway_is_plan() {
  [[ "${TAXIWAY_EXECUTION_MODE:-apply}" == "plan" ]]
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
