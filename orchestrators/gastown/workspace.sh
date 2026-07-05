#!/usr/bin/env bash
# workspace.sh — workspace phase for gastown.
# Registers a rig and creates a crew member via Gas Town CLI.
# No-op when TAXIWAY_REPO_URL is not set.
#
# Environment variables consumed:
#   TAXIWAY_REPO_URL     (required to activate)
#   TAXIWAY_REPO_REF     (optional) — ref to checkout in the refinery after rig add
#   TAXIWAY_REPO_PATH    (optional) — ignored for gastown; emits a warning
#   TAXIWAY_RIG_NAME     (required) — rig name (= repoBasename)
#   TAXIWAY_CREW_NAME    (required) — crew member name (= lab name)
#   TAXIWAY_HQ_DIR       — Gas Town HQ directory (default /lab/work/gt)
#
# Environment variables exported (set for downstream phases):
#   TAXIWAY_WORKSPACE_DIR — crew working directory ($TAXIWAY_HQ_DIR/<rig>/crew/<crew>/)

set -euo pipefail

export PATH="$HOME/.local/bin:$PATH"

# shellcheck source=../../infra/trace/events.sh
source "$(dirname "${BASH_SOURCE[0]}")/../../infra/trace/events.sh" 2>/dev/null || true
# shellcheck source=../../infra/workspace/clone.sh
source "$(dirname "${BASH_SOURCE[0]}")/../../infra/workspace/clone.sh"
lab_emit_event phase start

if [[ -z "${TAXIWAY_REPO_URL:-}" ]]; then
    echo "No repo configured for this lab — skipping workspace phase"
    lab_emit_event phase done
    exit 0
fi

# If Taxiway prepared an isolated workspace repo, use it as the fallback Git
# remote. Prefer TAXIWAY_RIG_SOURCE_URL below when available so Gas Town can
# create the rig from the lab-local bare repository.
if [[ -n "${TAXIWAY_REPO_FORK_URL:-}" ]]; then
    TAXIWAY_REPO_URL="$TAXIWAY_REPO_FORK_URL"
    export TAXIWAY_REPO_URL
fi

: "${TAXIWAY_RIG_NAME:?TAXIWAY_RIG_NAME is required for gastown workspace}"
: "${TAXIWAY_CREW_NAME:?TAXIWAY_CREW_NAME is required for gastown workspace}"

# Defensive guard: names must be sanitised by buildBaseEnv before reaching here.
LC_ALL=C  # force byte-wise character class comparison in case statements
for _var in TAXIWAY_RIG_NAME TAXIWAY_CREW_NAME; do
    _val="${!_var:-}"
    case "$_val" in
        ""|*[!A-Za-z0-9_]*)
            printf 'ERROR: %s=%q contains characters forbidden by Gas Town (expected sanitised by buildBaseEnv)\n' "$_var" "$_val" >&2
            exit 1 ;;
    esac
done
unset _var _val

# Warn about unsupported --repo-path for gastown.
if [[ -n "${TAXIWAY_REPO_PATH:-}" ]]; then
    echo "WARN: --repo-path is ignored for gastown (sessions are rooted at crew/<name>/)" >&2
fi

HQ_DIR="${TAXIWAY_HQ_DIR:-/lab/work/gt}"
MARKER="$HQ_DIR/.taxiway-hq-initialized"

prepare_beads_dir() {
    mkdir -p "$HQ_DIR/.beads"
    chmod 700 "$HQ_DIR/.beads"
}

install_town_settings() {
    if [[ "${TAXIWAY_ORCH_PROFILE_CLEAR:-false}" == "true" ]]; then
        rm -f "$HQ_DIR/settings/config.json"
        return
    fi
    if [[ -f "${TAXIWAY_ORCH_PROFILE_DIR:-}/settings/config.json" ]]; then
        mkdir -p "$HQ_DIR/settings"
        cp "$TAXIWAY_ORCH_PROFILE_DIR/settings/config.json" "$HQ_DIR/settings/config.json"
    fi
}

# Initialisation paresseuse du HQ (workspace tourne AVANT start dans le pipeline)
if [[ ! -f "$MARKER" ]]; then
    echo "Initializing gastown HQ at $HQ_DIR (lazy init from workspace phase)"
    mkdir -p "$HQ_DIR"
    prepare_beads_dir
    gt install "$HQ_DIR" --git
    touch "$MARKER"
fi
install_town_settings

cd "$HQ_DIR"  # toutes les commandes gt ci-dessous héritent du cwd

# 1) Add the rig (idempotent: skip if already present).
if gt rig list 2>/dev/null | awk '{print $1}' | grep -Fxq "$TAXIWAY_RIG_NAME"; then
    # Collision check: verify the existing rig points to the same repo URL
    existing_url=$(git -C "$HQ_DIR/$TAXIWAY_RIG_NAME/refinery/rig" remote get-url origin 2>/dev/null || true)
    if [[ -n "$existing_url" && "$existing_url" != "$TAXIWAY_REPO_URL" ]]; then
        echo "ERROR: rig '$TAXIWAY_RIG_NAME' already exists but points to '$existing_url', not '$TAXIWAY_REPO_URL' — rig name collision after sanitisation" >&2
        exit 1
    fi
    echo "Rig '$TAXIWAY_RIG_NAME' already registered — skipping 'gt rig add'"
else
    rig_source="${TAXIWAY_RIG_SOURCE_URL:-$TAXIWAY_REPO_URL}"
    echo "Adding rig '$TAXIWAY_RIG_NAME' from $rig_source"
    # Configure git identity if not set (required by gt rig add internal commits)
    if [[ -z "$(git config --global user.email 2>/dev/null)" ]]; then
        git config --global user.email "${TAXIWAY_CREW_NAME}@taxiway.local"
        git config --global user.name "${TAXIWAY_CREW_NAME}"
    fi
    # gt rig add clones the repo into ~/gt/<name>/refinery/rig/
    gt rig add "$TAXIWAY_RIG_NAME" "$rig_source"
fi

# 2) Checkout the requested ref if provided.
if [[ -n "${TAXIWAY_REPO_REF:-}" ]]; then
    refinery_dir="$HQ_DIR/$TAXIWAY_RIG_NAME/refinery/rig"
    if [[ -d "$refinery_dir/.git" ]]; then
        git -c protocol.file.allow=never -C "$refinery_dir" fetch --all --prune
        # Note: for a SHA or tag this produces a detached HEAD — intentional
        # for reproducible benchmark runs.
        git -c protocol.file.allow=never -C "$refinery_dir" checkout "$TAXIWAY_REPO_REF"
    else
        echo "WARN: refinery dir not found at $refinery_dir — skipping ref checkout" >&2
    fi
fi

# 3) Provision the crew workspace (idempotent).
# `gt crew add` creates the crew directory under ~/gt/<rig>/crew/<name>/.
# `gt crew start` launches an interactive Claude agent and must NOT be called
# here — that is the responsibility of start.sh (tmux session launch).
if gt crew list --rig "$TAXIWAY_RIG_NAME" 2>/dev/null | awk '{print $1}' | grep -Fxq "$TAXIWAY_CREW_NAME"; then
    echo "Crew '$TAXIWAY_CREW_NAME' already exists — skipping 'gt crew add'"
else
    echo "Adding crew workspace '$TAXIWAY_CREW_NAME' for rig '$TAXIWAY_RIG_NAME'"
    gt crew add "$TAXIWAY_CREW_NAME" --rig "$TAXIWAY_RIG_NAME"
fi

# 4) Export TAXIWAY_WORKSPACE_DIR using HQ_DIR (resolved above from TAXIWAY_HQ_DIR or auto-detected default).
export TAXIWAY_WORKSPACE_DIR="$HQ_DIR/$TAXIWAY_RIG_NAME/crew/$TAXIWAY_CREW_NAME"
echo "TAXIWAY_WORKSPACE_DIR=$TAXIWAY_WORKSPACE_DIR"

lab_emit_event phase done
