#!/usr/bin/env bash
set -euo pipefail

export PATH="$HOME/.local/bin:$PATH"

if [ -f "${HOME}/.config/taxiway/env" ]; then
    set -a
    # shellcheck disable=SC1091
    . "${HOME}/.config/taxiway/env"
    set +a
fi

HQ_DIR="${TAXIWAY_HQ_DIR:-/lab/work/gt}"
FORCE="${TAXIWAY_FORCE:-false}"
MARKER="$HQ_DIR/.taxiway-hq-initialized"
GASTOWN_LITELLM_AGENT="claude-code-litellm"
GASTOWN_MODEL="${TAXIWAY_SET_MODEL:-claude-opus-4-8}"
TAXIWAY_LITELLM_BASE_URL="${TAXIWAY_LITELLM_BASE_URL:-http://${TAXIWAY_LAB:-lab}.litellm.localhost:4000}"

prepare_beads_dir() {
    mkdir -p "$HQ_DIR/.beads"
    chmod 700 "$HQ_DIR/.beads"
}

install_town_settings() {
    if [[ "${TAXIWAY_ORCH_PROFILE_CLEAR:-false}" == "true" ]]; then
        rm -f "$HQ_DIR/settings/config.json"
        rm -f "$HQ_DIR/settings/agents.json"
        return
    fi

    local file
    for file in config.json agents.json; do
        if [[ -f "${TAXIWAY_ORCH_PROFILE_DIR:-}/settings/$file" ]]; then
            mkdir -p "$HQ_DIR/settings"
            cp "$TAXIWAY_ORCH_PROFILE_DIR/settings/$file" "$HQ_DIR/settings/$file"
        fi
    done
}

require_litellm_env() {
    if [ -z "${TAXIWAY_LITELLM_API_KEY:-}" ]; then
        printf '  \033[1;31mERROR\033[0m LiteLLM is required for Gas Town but TAXIWAY_LITELLM_API_KEY is missing\n'
        printf '        From the host, run: taxiway gateway %s\n' "${TAXIWAY_LAB:-<lab>}"
        exit 1
    fi
}

configure_litellm_town_settings() {
    require_litellm_env
    mkdir -p "$HQ_DIR/settings"
    GASTOWN_SETTINGS_DIR="$HQ_DIR/settings" \
    GASTOWN_LITELLM_AGENT="$GASTOWN_LITELLM_AGENT" \
    GASTOWN_MODEL="$GASTOWN_MODEL" \
    TAXIWAY_LITELLM_BASE_URL="$TAXIWAY_LITELLM_BASE_URL" \
    TAXIWAY_LITELLM_API_KEY="$TAXIWAY_LITELLM_API_KEY" \
    CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC="${CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC:-1}" \
    python3 - <<'PY'
import json
import os
from pathlib import Path

settings_dir = Path(os.environ["GASTOWN_SETTINGS_DIR"])
config_path = settings_dir / "config.json"
agents_path = settings_dir / "agents.json"
agent_name = os.environ["GASTOWN_LITELLM_AGENT"]
model = os.environ["GASTOWN_MODEL"]
base_url = os.environ["TAXIWAY_LITELLM_BASE_URL"].rstrip("/")
api_key = os.environ["TAXIWAY_LITELLM_API_KEY"]


def load_json(path):
    if not path.exists():
        return {}
    with path.open(encoding="utf-8") as fh:
        return json.load(fh)


def write_json(path, value):
    with path.open("w", encoding="utf-8") as fh:
        json.dump(value, fh, indent=2)
        fh.write("\n")


town_settings = load_json(config_path)
town_settings.setdefault("type", "town-settings")
town_settings.setdefault("version", 1)
town_settings["default_agent"] = agent_name

role_agents = town_settings.get("role_agents")
if not isinstance(role_agents, dict):
    role_agents = {}
for role in ("boot", "crew", "deacon", "dog", "mayor", "polecat", "refinery", "witness"):
    role_agents[role] = agent_name
town_settings["role_agents"] = role_agents
write_json(config_path, town_settings)

agent_registry = load_json(agents_path)
agent_registry.setdefault("version", 1)
agents = agent_registry.get("agents")
if not isinstance(agents, dict):
    agents = {}
agents[agent_name] = {
    "provider": "claude",
    "command": "claude",
    "args": [
        "--model",
        model,
        "--dangerously-skip-permissions",
    ],
    "env": {
        "ANTHROPIC_BASE_URL": base_url,
        "ANTHROPIC_CUSTOM_HEADERS": f"x-litellm-api-key: Bearer {api_key}",
        "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": os.environ["CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC"],
    },
}
agent_registry["agents"] = agents
write_json(agents_path, agent_registry)
PY
}

start_dashboard() {
    local dashboard_host_port dashboard_port_file dashboard_url
    DASHBOARD_SESSION="dashboard"
    dashboard_host_port="${TAXIWAY_DASHBOARD_HOST_PORT:-}"
    dashboard_port_file="$HQ_DIR/.runtime/dashboard.port"
    if [[ -z "$dashboard_host_port" && -f "$dashboard_port_file" ]]; then
        dashboard_host_port="$(<"$dashboard_port_file")"
    fi
    dashboard_url=""
    if [[ -n "$dashboard_host_port" ]]; then
        dashboard_url="http://127.0.0.1:$dashboard_host_port"
    fi

    if tmux has-session -t "$DASHBOARD_SESSION" 2>/dev/null; then
        echo "Dashboard tmux session '$DASHBOARD_SESSION' already running"
        if [[ -n "$dashboard_url" ]]; then
            echo "Dashboard: $dashboard_url"
        fi
        return
    fi

    mkdir -p "$HQ_DIR/.runtime"
    if [[ -n "$dashboard_host_port" ]]; then
        printf '%s\n' "$dashboard_host_port" > "$dashboard_port_file"
    fi
    tmux new-session -d -s "$DASHBOARD_SESSION" -c "$HQ_DIR" "gt dashboard --bind 127.0.0.1"

    if [[ -n "$dashboard_url" ]]; then
        echo "Dashboard: $dashboard_url"
    fi
}

start_feed() {
    FEED_SESSION="feed"

    if tmux has-session -t "$FEED_SESSION" 2>/dev/null; then
        echo "Feed tmux session '$FEED_SESSION' already running"
        return
    fi

    tmux new-session -d -s "$FEED_SESSION" -c "$HQ_DIR" "gt feed"
    echo "Feed tmux session '$FEED_SESSION' created"
}

run_startup_doctor_fix() {
    local log_file
    log_file="$HQ_DIR/.runtime/doctor-fix.log"
    mkdir -p "$HQ_DIR/.runtime"

    if gt doctor --fix --no-start >"$log_file" 2>&1; then
        return
    fi

    echo "WARN: gt doctor --fix --no-start reported issues during startup; continuing" >&2
    sed 's/^/  /' "$log_file" >&2 || true
}

start_gastown() {
    local status

    # Gas Town's gt up has a very short daemon readiness check. Start the daemon
    # first and wait for its initial heartbeat so gt up does not race it.
    ensure_gastown_daemon_ready

    if gt up; then
        return 0
    else
        status=$?
        gt daemon logs >&2 || true
        return "$status"
    fi
}

ensure_gastown_daemon_ready() {
    local log_lines

    if gastown_daemon_has_heartbeat; then
        return 0
    fi

    log_lines="$(gastown_daemon_log_line_count)"
    if ! gastown_daemon_is_running; then
        gt daemon start || true
        if ! gastown_daemon_is_running; then
            gt daemon logs >&2 || true
            return 1
        fi
    fi

    wait_for_gastown_daemon_heartbeat "$log_lines"
}

gastown_daemon_is_running() {
    local status
    status="$(gt daemon status 2>&1 || true)"

    if [[ "$status" == *"not running"* ]]; then
        return 1
    fi
    [[ "$status" == *"Daemon is"* && "$status" == *"running"* ]]
}

gastown_daemon_has_heartbeat() {
    local status
    status="$(gt daemon status 2>&1 || true)"

    [[ "$status" == *"Last heartbeat:"* ]]
}

gastown_daemon_log_line_count() {
    { gt daemon logs -n 1000 2>/dev/null || true; } | wc -l | tr -d '[:space:]'
}

wait_for_gastown_daemon_heartbeat() {
    local start_line="${1:-0}"
    local timeout="${TAXIWAY_GASTOWN_DAEMON_HEARTBEAT_TIMEOUT_SECONDS:-60}"
    local deadline=$((SECONDS + timeout))

    while (( SECONDS < deadline )); do
        if gastown_daemon_has_heartbeat; then
            return 0
        fi
        if { gt daemon logs -n 1000 2>/dev/null || true; } | tail -n "+$((start_line + 1))" | grep -q "Heartbeat complete"; then
            return 0
        fi
        sleep 1
    done

    echo "ERROR: timed out waiting for Gas Town daemon initial heartbeat" >&2
    gt daemon logs >&2 || true
    return 1
}

source "$(dirname "$0")/../../infra/trace/events.sh" 2>/dev/null || true
lab_emit_event phase start

require_litellm_env

# Idempotence check
if [[ -f "$MARKER" ]] && [[ "$FORCE" != "true" ]]; then
    echo "HQ already initialized at $HQ_DIR (use --force to reinitialize)"
else
    # Force: remove existing HQ
    if [[ "$FORCE" == "true" ]] && [[ -d "$HQ_DIR" ]]; then
        echo "Removing existing HQ at $HQ_DIR"
        rm -rf "$HQ_DIR"
    fi

    mkdir -p "$HQ_DIR"
    prepare_beads_dir

    echo "Initializing gastown HQ at $HQ_DIR"
    gt install "$HQ_DIR" --git

    # Write marker
    touch "$MARKER"
    echo "HQ initialized at $HQ_DIR"
fi
install_town_settings
configure_litellm_town_settings

cd "$HQ_DIR"
rm -f "$HQ_DIR/.claude/settings.json"
start_gastown
if [[ -n "${TAXIWAY_RIG_NAME:-}" ]]; then
    gt crew start --rig "$TAXIWAY_RIG_NAME"
fi
gt enable
gt shell install
run_startup_doctor_fix
gt status
start_feed
start_dashboard

SESSION="gastown"
tmux kill-session -t "$SESSION" 2>/dev/null || true

start_dir="$HQ_DIR"
if [[ -n "${TAXIWAY_RIG_NAME:-}" && -n "${TAXIWAY_CREW_NAME:-}" ]]; then
    crew_dir="$HQ_DIR/$TAXIWAY_RIG_NAME/crew/$TAXIWAY_CREW_NAME"
    if [[ -d "$crew_dir" ]]; then
        start_dir="$crew_dir"
    fi
fi

tmux new-session -d -s "$SESSION" -c "$start_dir" "${SHELL:-/bin/bash}" -l
echo "tmux session '$SESSION' created (cwd: $start_dir)"

lab_emit_event phase done
