#!/usr/bin/env bash
# start.sh — start phase for gascity.
#
# Creates the city when missing, routes every Claude agent through the lab's
# LiteLLM gateway with a Taxiway-managed config fragment, restarts the Gas City
# supervisor so it carries the current lab environment, waits for the city's
# agent sessions and opens the `gascity` workspace shell session.

set -euo pipefail

export PATH="$HOME/.local/bin:$PATH"

# shellcheck source=../../infra/commands/steps.sh
source "$(dirname "${BASH_SOURCE[0]}")/../../infra/commands/steps.sh"
# shellcheck source=lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

log() { printf '\n\033[1;34m[gascity-start]\033[0m %s\n' "$*"; }

if [ -f "${HOME}/.config/taxiway/env" ]; then
    set -a
    # shellcheck disable=SC1091
    . "${HOME}/.config/taxiway/env"
    set +a
fi

if ! taxiway_is_plan; then
    # shellcheck source=../../agents/claude-code/env.sh
    source "$(dirname "${BASH_SOURCE[0]}")/../../agents/claude-code/env.sh"
    claude_code_write_env "${TAXIWAY_SET_TOOL_SEARCH:-true}" "${TAXIWAY_SET_CLAUDEAI_MCP_SERVERS:-false}"
fi

FORCE="${TAXIWAY_FORCE:-false}"
SESSION="gascity"
FRAGMENT="taxiway.toml"
GASCITY_MODEL="${TAXIWAY_SET_MODEL:?Missing model: start this orchestrator through Taxiway}"
TAXIWAY_LITELLM_BASE_URL="${TAXIWAY_LITELLM_BASE_URL:-http://${TAXIWAY_LAB:-lab}.litellm.internal:4000}"
READY_TIMEOUT="${TAXIWAY_GASCITY_READY_TIMEOUT_SECONDS:-240}"
# Gas City's official runtime differs by environment: its container images run
# the supervisor in the foreground under an init, while `gc start` installs a
# systemd user unit on a VM. A container guest has no user manager.
GUEST_KIND="${TAXIWAY_GUEST_KIND:-vm}"

RIG_ROOT=""
if [[ -n "${TAXIWAY_REPO_URL:-}" && -n "${TAXIWAY_WORKSPACE_DIR:-}" ]]; then
    RIG_ROOT="$(gascity_rig_root)"
fi

if taxiway_is_plan; then
    log "Configuring Claude Code tool search and MCP settings"
    taxiway_plan_detail "tool search: ${TAXIWAY_SET_TOOL_SEARCH:-true}"
    taxiway_plan_detail "Claude AI MCP servers: ${TAXIWAY_SET_CLAUDEAI_MCP_SERVERS:-false}"
    if [[ "$FORCE" == "true" ]]; then
        log "Removing the existing Gas City city at $GASCITY_CITY_DIR"
    fi
    if taxiway_can_inspect && [[ -f "$GASCITY_CITY_DIR/city.toml" ]] && [[ "$FORCE" != "true" ]]; then
        log "Gas City city already initialized at $GASCITY_CITY_DIR"
    else
        log "Initializing Gas City city at $GASCITY_CITY_DIR when required"
        taxiway_plan_detail "template: $GASCITY_TEMPLATE"
    fi
    if [[ -n "$RIG_ROOT" ]]; then
        log "Registering rig from $RIG_ROOT when missing"
    fi
    log "Configuring Gas City for the Taxiway LiteLLM gateway"
    taxiway_plan_detail "model: $GASCITY_MODEL"
    taxiway_plan_detail "effort: $GASCITY_EFFORT"
    taxiway_plan_detail "fragment: $GASCITY_CITY_DIR/$FRAGMENT"
    log "Trusting the Gas City city for Claude Code"
    log "Restarting the Gas City supervisor"
    if [[ "$GUEST_KIND" == container ]]; then
        taxiway_plan_detail "container guest: detached gc supervisor start, then gc start"
    else
        taxiway_plan_detail "vm guest: gc start with its systemd user unit"
    fi
    log "Waiting for Gas City agent sessions"
    if taxiway_can_inspect && tmux has-session -t "$SESSION" 2>/dev/null; then
        log "Stopping existing tmux session '$SESSION'"
    fi
    log "Starting tmux session '$SESSION'"
    exit 0
fi

source "$(dirname "$0")/../../infra/trace/events.sh" 2>/dev/null || true
lab_emit_event phase start

if [ -z "${TAXIWAY_LITELLM_API_KEY:-}" ]; then
    printf '  \033[1;31mERROR\033[0m LiteLLM is required for Gas City but TAXIWAY_LITELLM_API_KEY is missing\n'
    printf '        From the host, run: taxiway gateway %s\n' "${TAXIWAY_LAB:-<lab>}"
    exit 1
fi

# Exports the managed Claude settings and selects the auth mode: api-key exports
# ANTHROPIC_AUTH_TOKEN, subscription unsets it so the supervisor cannot leak a
# stale token into agent sessions.
claude_code_load_env

# The restarted supervisor must carry the environment and gateway
# configuration loaded above; reusing a running one would keep stale ones.
if [[ "$FORCE" == "true" && -d "$GASCITY_CITY_DIR" ]]; then
    echo "Removing existing city at $GASCITY_CITY_DIR"
    gascity_stop_city
    rm -rf "$GASCITY_CITY_DIR"
fi

gascity_ensure_city
if [[ -n "$RIG_ROOT" && -d "$RIG_ROOT/.git" ]]; then
    gascity_ensure_rig
fi

# write_gateway_fragment renders the Taxiway-managed fragment and includes it
# from city.toml. Rewriting the whole file keeps restarts idempotent: TOML
# tables cannot repeat, so nothing is appended to city.toml itself. The effort
# is a Claude provider option default, which Gas City only reads from
# city.toml: that single key is updated in place.
write_gateway_fragment() {
    GASCITY_MODEL="$GASCITY_MODEL" \
    GASCITY_EFFORT="$GASCITY_EFFORT" \
    GASCITY_FRAGMENT="$FRAGMENT" \
    TAXIWAY_LITELLM_BASE_URL="$TAXIWAY_LITELLM_BASE_URL" \
    python3 - "$GASCITY_CITY_DIR" <<'PY'
import copy
import json
import os
import re
import stat
import sys
import tempfile
import tomllib
from pathlib import Path

city = Path(sys.argv[1])
fragment_name = os.environ["GASCITY_FRAGMENT"]
api_key_mode = os.environ.get("TAXIWAY_CLAUDE_AUTH_MODE", "subscription") == "api-key"


def q(value):
    # JSON string escapes are valid TOML basic-string escapes.
    return json.dumps(value)


def literal(name, value):
    # Gas City expands $VAR in config values; only secret refs may use them.
    if "$" in value:
        raise SystemExit(f"{name} must not contain '$'")
    return q(value)


# Claude settings Taxiway manages for every launch, including restarts and
# sessions Gas City creates later.
claude_env = {
    "ENABLE_TOOL_SEARCH": os.environ["ENABLE_TOOL_SEARCH"],
    "ENABLE_CLAUDEAI_MCP_SERVERS": os.environ["ENABLE_CLAUDEAI_MCP_SERVERS"],
    "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": os.environ.get("CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC", "1"),
}
for name in ("ANTHROPIC_DEFAULT_OPUS_MODEL", "ANTHROPIC_DEFAULT_SONNET_MODEL",
             "ANTHROPIC_DEFAULT_HAIKU_MODEL", "ANTHROPIC_DEFAULT_FABLE_MODEL",
             "TAXIWAY_CLAUDE_AVAILABLE_MODELS", "DISABLE_AUTOUPDATER"):
    if os.environ.get(name):
        claude_env[name] = os.environ[name]

lines = [
    "# Managed by Taxiway: rewritten by every `taxiway start`. Do not edit.",
    "",
    "[upstreams.taxiway-litellm]",
    'description = "Taxiway LiteLLM gateway"',
    f"base_url = {literal('base_url', os.environ['TAXIWAY_LITELLM_BASE_URL'].rstrip('/'))}",
    # Gas City 1.5.0 drops the builtin claude upstream_env binding for
    # base = "builtin:claude" providers; name the variables explicitly until a
    # release with gastownhall/gascity#5985 is pinned (tracked in #178).
    'base_url_env = "ANTHROPIC_BASE_URL"',
]
if api_key_mode:
    lines += [
        'auth_token = "$TAXIWAY_LITELLM_API_KEY"',
        'auth_token_env = "ANTHROPIC_AUTH_TOKEN"',
    ]
lines += [
    "",
    "[upstreams.taxiway-litellm.env]",
    'ANTHROPIC_CUSTOM_HEADERS = "x-litellm-api-key: Bearer $TAXIWAY_LITELLM_API_KEY"',
    "",
    "[agent_defaults]",
    'upstream = "taxiway-litellm"',
    "",
    "[providers.claude]",
    f"args = [\"--model\", {literal('model', os.environ['GASCITY_MODEL'])}]",
    "",
    "[providers.claude.env]",
]
lines += [f"{name} = {literal(name, value)}" for name, value in sorted(claude_env.items())]
content = "\n".join(lines) + "\n"
tomllib.loads(content)



def write_atomically(path, text, mode):
    fd, tmp = tempfile.mkstemp(prefix=".taxiway-", dir=path.parent)
    try:
        with os.fdopen(fd, "w") as out:
            out.write(text)
        os.chmod(tmp, mode)
        os.replace(tmp, path)
    finally:
        if os.path.exists(tmp):
            os.unlink(tmp)


write_atomically(city / fragment_name, content, 0o600)

config_path = city / "city.toml"
original = config_path.read_text()
data = tomllib.loads(original)
expected = copy.deepcopy(data)
text = original

# Include the fragment once.
if fragment_name not in data.get("include", []):
    if "include" not in data:
        text = f"include = [{q(fragment_name)}]\n" + text
    else:
        match = re.search(r"(?m)^include\s*=\s*\[([^\]\n]*)\]([ \t]*#[^\n]*)?[ \t]*$", text)
        if not match:
            raise SystemExit(f"city.toml: include is not a single-line array; add {q(fragment_name)} to it")
        items = match.group(1).strip().rstrip(",").strip()
        text = (text[:match.start()] + f"include = [{items + ', ' if items else ''}{q(fragment_name)}]"
                + (match.group(2) or "") + text[match.end():])
    expected["include"] = [*data.get("include", []), fragment_name]

# Set effort among the Claude provider's option defaults, keeping other keys.
claude = data.get("providers", {}).get("claude")
if not isinstance(claude, dict):
    raise SystemExit("city.toml has no [providers.claude] table")
defaults = dict(claude.get("option_defaults", {}))
defaults["effort"] = os.environ["GASCITY_EFFORT"]
expected["providers"]["claude"]["option_defaults"] = defaults
lines = text.splitlines(keepends=True)
header = re.compile(r"^\s*\[")
def table_range(pattern):
    starts = [i for i, line in enumerate(lines) if re.match(pattern, line)]
    if not starts:
        return None
    end = next((i for i in range(starts[0] + 1, len(lines)) if header.match(lines[i])), len(lines))
    return starts[0], end
subtable = table_range(r"^\s*\[providers\.claude\.option_defaults\]\s*(#.*)?$")
if subtable:
    del lines[subtable[0]:subtable[1]]
claude_table = table_range(r"^\s*\[providers\.claude\]\s*(#.*)?$")
if not claude_table:
    raise SystemExit("city.toml: [providers.claude] must be a table header to set the effort")
start, end = claude_table
lines[start + 1:end] = [line for line in lines[start + 1:end] if not re.match(r"^\s*option_defaults\s*[.=]", line)]
inline = ", ".join(f"{json.dumps(key) if not re.fullmatch(r'[A-Za-z0-9_-]+', key) else key} = {q(value)}"
                   for key, value in sorted(defaults.items()))
lines.insert(start + 1, f"option_defaults = {{ {inline} }}\n")
text = "".join(lines)

# Never write a config that changes anything but the include and option defaults.
if tomllib.loads(text) != expected:
    raise SystemExit("city.toml: refusing an edit that would change other settings")
if text != original:
    write_atomically(config_path, text, stat.S_IMODE(config_path.stat().st_mode))
PY
}

log "Configuring Gas City for the Taxiway LiteLLM gateway"
write_gateway_fragment

# Gas City agents work under the city (.gc/agents/*) and in the rig. Trusting
# both roots covers every agent directory, including ones created later.
log "Trusting the Gas City city for Claude Code"
trust_hook="$(dirname "${BASH_SOURCE[0]}")/../../agents/claude-code/trust-workspace.sh"
for root in "$GASCITY_CITY_DIR" ${RIG_ROOT:+"$RIG_ROOT"}; do
    TAXIWAY_WORKSPACE_TRUST_PATH="$root" bash "$trust_hook"
done

log "Restarting the Gas City supervisor"
# Close the shell session early: killing the default server's last session
# right before creating a new one races its exit ("server exited unexpectedly").
tmux kill-session -t "$SESSION" 2>/dev/null || true
gascity_stop_city
supervisor_failed() {
    tail -n 40 "$HOME/.gc/supervisor.log" >&2 2>/dev/null || true
    exit 1
}
if [[ "$GUEST_KIND" == container ]]; then
    # Detached `gc supervisor run` inherits this environment (the fragment's
    # $TAXIWAY_LITELLM_API_KEY references); no service unit is installed.
    echo "Starting the Gas City supervisor detached (container guest)"
    timeout 120 gc supervisor start || supervisor_failed
    echo "gc start may report that a per-user systemd instance is not available:" \
        "expected in a container guest, which uses the detached supervisor above"
else
    # `gc start` installs and starts Gas City's systemd user unit, which only
    # receives allow-listed variables: opt the gateway variables in for the
    # $TAXIWAY_LITELLM_API_KEY references in the fragment.
    export GC_SUPERVISOR_ENV="TAXIWAY_LITELLM_API_KEY TAXIWAY_LITELLM_BASE_URL"
fi
(cd "$GASCITY_CITY_DIR" && timeout 300 gc start "$GASCITY_CITY_DIR") || supervisor_failed

# wait_for_sessions waits until the city reports active agent sessions and no
# session is still pending creation.
wait_for_sessions() {
    local deadline=$((SECONDS + READY_TIMEOUT)) report=""
    while (( SECONDS < deadline )); do
        if report="$(cd "$GASCITY_CITY_DIR" && timeout 30 gc session list --json 2>/dev/null)" \
            && GASCITY_SESSIONS="$report" python3 - <<'PY'
import json, os, sys
sessions = json.loads(os.environ["GASCITY_SESSIONS"]).get("sessions") or []
states = [str(s.get("state", "")).lower() for s in sessions]
pending = {"creating", "start-pending"}
sys.exit(0 if "active" in states and not pending.intersection(states) else 1)
PY
        then
            echo "Gas City agent sessions are active"
            (cd "$GASCITY_CITY_DIR" && timeout 30 gc session list) || true
            return 0
        fi
        sleep 3
    done
    echo "ERROR: timed out after ${READY_TIMEOUT}s waiting for Gas City agent sessions" >&2
    (cd "$GASCITY_CITY_DIR" && timeout 30 gc session list) >&2 || true
    tail -n 40 "$HOME/.gc/supervisor.log" >&2 2>/dev/null || true
    return 1
}

log "Waiting for Gas City agent sessions"
wait_for_sessions
# Only Lima labs forward the dashboard to the host; Docker labs do not publish it.
if [[ "${TAXIWAY_GUEST_KIND:-}" == vm && -n "${TAXIWAY_DASHBOARD_HOST_PORT:-}" ]]; then
    echo "Dashboard: http://127.0.0.1:${TAXIWAY_DASHBOARD_HOST_PORT}"
fi

start_dir="$GASCITY_CITY_DIR"
if [[ -n "$RIG_ROOT" && -d "$RIG_ROOT" ]]; then
    start_dir="$RIG_ROOT"
fi
tmux new-session -d -s "$SESSION" -c "$start_dir" "${SHELL:-/bin/bash}" -l
echo "tmux session '$SESSION' created (cwd: $start_dir)"

lab_emit_event phase done
