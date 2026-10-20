#!/usr/bin/env bash
# Shared Gas City helpers for the workspace and start phases.
#
# The city lives at $GASCITY_CITY_DIR (default /lab/work/gc). Its workspace
# repository, when configured, is a rig rooted at the repository clone.

GASCITY_CITY_DIR="${TAXIWAY_CITY_DIR:-/lab/work/gc}"
GASCITY_TEMPLATE="${TAXIWAY_SET_TEMPLATE:-gastown}"

# Both templates define always-on sessions, which start readiness relies on.
case "$GASCITY_TEMPLATE" in
    gastown|minimal) ;;
    *)
        printf 'ERROR: unsupported Gas City template %q; use gastown or minimal\n' "$GASCITY_TEMPLATE" >&2
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
