#!/usr/bin/env bash
# Shared Gas City helpers for the workspace, start and stop phases.
#
# The city lives at $GASCITY_CITY_DIR (default /lab/work/gc). Its workspace
# repository, when configured, is a rig rooted at the repository clone.

GASCITY_CITY_DIR="${TAXIWAY_CITY_DIR:-/lab/work/gc}"
GASCITY_TEMPLATE="${TAXIWAY_SET_TEMPLATE:-gastown}"
GASCITY_EFFORT="${TAXIWAY_SET_EFFORT:-max}"

# Both templates define always-on sessions, which start readiness relies on.
case "$GASCITY_TEMPLATE" in
    gastown|minimal) ;;
    *)
        printf 'ERROR: unsupported Gas City template %q; use gastown or minimal\n' "$GASCITY_TEMPLATE" >&2
        exit 1 ;;
esac
# Gas City's builtin Claude effort tiers.
case "$GASCITY_EFFORT" in
    low|medium|high|xhigh|max) ;;
    *)
        printf 'ERROR: unsupported Gas City effort %q; use low, medium, high, xhigh or max\n' "$GASCITY_EFFORT" >&2
        exit 1 ;;
esac

# gascity_rig_root prints the repository clone used as the rig root. Gas City
# rigs are whole repositories, so --repo-path does not move the rig.
gascity_rig_root() {
    local dir="${TAXIWAY_WORKSPACE_DIR:-}"
    if [[ -n "${TAXIWAY_REPO_PATH:-}" ]]; then
        dir="${dir%/"$TAXIWAY_REPO_PATH"}"
    fi
    printf '%s\n' "$dir"
}

# gascity_rig_name prints the rig name derived from the repository directory.
gascity_rig_name() {
    local name
    name="$(basename "$(gascity_rig_root)")"
    if [[ ! "$name" =~ ^[A-Za-z0-9._-]{1,64}$ || "$name" =~ ^[0-9]+$ ]]; then
        printf 'ERROR: repository name %q is not a valid Gas City rig name\n' "$name" >&2
        return 1
    fi
    printf '%s\n' "$name"
}

# gascity_ensure_city creates the city non-interactively when it is missing.
# The template only applies to a new city; an existing city keeps its own.
gascity_ensure_city() {
    if [[ -f "$GASCITY_CITY_DIR/city.toml" ]]; then
        echo "Gas City city already initialized at $GASCITY_CITY_DIR"
        return 0
    fi
    # Never adopt a half-initialized directory: gc init refuses to overwrite.
    rm -rf "$GASCITY_CITY_DIR"
    mkdir -p "$(dirname "$GASCITY_CITY_DIR")"
    echo "Initializing Gas City city at $GASCITY_CITY_DIR (template: $GASCITY_TEMPLATE)"
    gc init "$GASCITY_CITY_DIR" --template "$GASCITY_TEMPLATE" \
        --default-provider claude --providers claude \
        --yes --no-start --skip-provider-readiness
}

# gascity_stop_city stops the city, the machine-wide supervisor and the
# deferred-nudge pollers, which outlive the city and keep its files open.
# Without a running supervisor there is nothing to stop: `gc stop` would only
# wait. A supervisor that survives one bounded retry is an error in both guest
# kinds.
gascity_stop_city() {
    local stop_error=""
    if gc supervisor status >/dev/null 2>&1; then
        if [[ -f "$GASCITY_CITY_DIR/city.toml" ]]; then
            (cd "$GASCITY_CITY_DIR" && timeout 180 gc stop "$GASCITY_CITY_DIR") >/dev/null 2>&1 || true
        fi
        stop_error="$(timeout 120 gc supervisor stop --wait 2>&1 >/dev/null)" || true
        if gc supervisor status >/dev/null 2>&1; then
            echo "Gas City supervisor still running; retrying the stop once" >&2
            stop_error="$(timeout 120 gc supervisor stop --wait 2>&1 >/dev/null)" || true
            if gc supervisor status >/dev/null 2>&1; then
                printf 'ERROR: could not stop the running Gas City supervisor\n%s\n' "$stop_error" >&2
                return 1
            fi
        fi
    fi
    pkill -u "$(id -u)" -f 'gc nudge poll' 2>/dev/null || true
}

# gascity_rig_path <name> prints the registered path of a rig, if any.
gascity_rig_path() {
    (cd "$GASCITY_CITY_DIR" && timeout 30 gc rig list --json 2>/dev/null) \
        | jq -r --arg name "$1" '(.rigs // .)[]? | select(.name == $name) | .path // empty' 2>/dev/null \
        | head -n 1
}

# gascity_ensure_rig registers the workspace repository as a rig (idempotent).
gascity_ensure_rig() {
    local root name existing
    root="$(gascity_rig_root)"
    name="$(gascity_rig_name)" || return 1
    [[ -d "$root/.git" ]] || { echo "ERROR: workspace repository not found at $root" >&2; return 1; }
    existing="$(gascity_rig_path "$name")"
    if [[ -n "$existing" ]]; then
        if [[ "$(cd "$existing" 2>/dev/null && pwd -P)" != "$(cd "$root" && pwd -P)" ]]; then
            echo "ERROR: rig '$name' already exists at '$existing', not '$root'" >&2
            return 1
        fi
        echo "Rig '$name' already registered — skipping 'gc rig add'"
        return 0
    fi
    echo "Adding rig '$name' from $root"
    (cd "$GASCITY_CITY_DIR" && gc rig add "$root" --name "$name")
}
