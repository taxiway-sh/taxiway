"""Opt-in live Claude tests. Run with an authenticated reference lab.

Uses real subscription requests; never prints or stores OAuth credentials.
The reference lab is preserved. A second, temporary Docker lab is removed.
"""

import argparse
import json
import os
from pathlib import Path
import shlex
import subprocess
import sys
import uuid


def command(argv, *, data=None, timeout=300):
    result = subprocess.run(argv, input=data, stdout=subprocess.PIPE,
                            stderr=subprocess.PIPE, timeout=timeout)
    if result.returncode:
        # Do not echo subprocess output: auth/proxy errors may contain secrets.
        raise RuntimeError(f"{argv[0]} exited {result.returncode}")
    return result.stdout


def guest(lab, script, *, data=None, timeout=300):
    state = Path(os.environ["TAXIWAY_LAB_STATE_DIR"])
    ref = json.loads((state / lab / "ref.json").read_text())
    runtime = f"taxiway-{os.environ['TAXIWAY_CONTEXT']}-{os.environ['TAXIWAY_CONTEXT_ID']}-{lab}"
    if ref["driver"] == "docker":
        argv = ["docker", "exec", "-i", "-u", "taxiway", "-e",
                "HOME=/home/taxiway", "-w", "/lab/work", runtime]
    elif ref["driver"] == "lima":
        argv = ["limactl", "shell", "--workdir=/lab/work", runtime]
    else:
        raise RuntimeError("Unsupported reference lab driver")
    return command(argv + ["bash", "-lc", script], data=data, timeout=timeout)


def run_claude(lab, model, prompt, agents=None):
    argv = ["claude", "-p", prompt, "--model", model,
            "--output-format", "stream-json", "--verbose",
            "--forward-subagent-text", "--no-session-persistence",
            "--effort", "low", "--permission-mode", "dontAsk",
            "--tools", "Agent" if agents else "",
            "--allowedTools", "Agent", "--max-budget-usd", "2"]
    if agents:
        argv += ["--agents", json.dumps(agents)]
    script = """set -euo pipefail
set -a
source "$HOME/.config/taxiway/env"
set +a
source /lab/agents/claude-code/env.sh
claude_code_load_env
export ANTHROPIC_BASE_URL="${TAXIWAY_LITELLM_BASE_URL%/}"
export ANTHROPIC_CUSTOM_HEADERS="x-litellm-api-key: Bearer ${TAXIWAY_LITELLM_API_KEY}"
export CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1
export CLAUDE_AGENT_SDK_DISABLE_BUILTIN_AGENTS=1
unset CLAUDE_CODE_SUBAGENT_MODEL CLAUDE_CODE_SUBAGENT_MODEL_FORCE
exec timeout --kill-after=10s 180s """ + shlex.join(argv)
    output = guest(lab, script)
    events = [json.loads(line) for line in output.splitlines() if line.startswith(b"{")]
    results = [e for e in events if e.get("type") == "result"]
    assert results, "Client produced no result event"
    result = results[-1]
    assert not result.get("is_error"), "Client reported an error"
    assert result.get("subtype") == "success", "Client did not finish successfully"
    assert not result.get("permission_denials"), "A tool was denied"
    models = set()
    spawned = set()
    nested_models = set()
    for event in events:
        for name, usage in event.get("modelUsage", {}).items():
            if usage.get("outputTokens", 0) > 0:
                models.add(name)
        message = event.get("message", {})
        if event.get("type") == "assistant":
            if message.get("model"):
                models.add(message["model"])
                if event.get("parent_tool_use_id"):
                    nested_models.add(message["model"])
            for block in message.get("content", []):
                if block.get("type") == "tool_use" and block.get("name") == "Agent":
                    spawned.add(block.get("input", {}).get("subagent_type"))
    return result.get("result", ""), models, spawned, nested_models


def check(lab, label, model, expected, prompt, agents=None):
    answer, models, spawned, nested = run_claude(lab, model, prompt, agents)
    assert "TAXIWAY_MODEL_OK" in answer, "Expected answer marker missing"
    assert expected <= models, f"Expected models {sorted(expected)}; observed {sorted(models)}"
    if agents:
        assert set(agents) <= spawned, f"Expected agent invocations {sorted(agents)}; observed {sorted(spawned)}"
        assert expected <= models
        assert nested, "No forwarded subagent response observed"
    else:
        assert not spawned, "Unexpected subagent invocation"
    print(f"PASS {label}: models={','.join(sorted(models))}; subagents={','.join(sorted(spawned))}", flush=True)
    return nested


def worker(model):
    return {"description": "A Taxiway test worker. Invoke when explicitly requested.",
            "prompt": "Reply exactly TAXIWAY_MODEL_OK. Do not use any tools.",
            "tools": [], "model": model}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--auth-lab", required=True)
    parser.add_argument("--taxiway", default="./taxiway")
    args = parser.parse_args()
    for key in ("TAXIWAY_CONTEXT", "TAXIWAY_CONTEXT_ID", "TAXIWAY_LAB_STATE_DIR"):
        if not os.environ.get(key):
            parser.error(f"{key} must be set; run through direnv exec .")
    if os.environ["TAXIWAY_CONTEXT"] not in ("dev", "e2e"):
        parser.error("Live tests require a dev or e2e context")
    guest(args.auth_lab, 'test -s "$HOME/.claude/.credentials.json"')
    # Resolve expectations from the same catalog used by Taxiway, without PyYAML.
    defaults = json.loads(guest(args.auth_lab, "source /lab/agents/claude-code/env.sh; "
                              "claude_code_load_env; python3 -c 'import json, os; "
                              "print(json.dumps({k:os.environ[\"ANTHROPIC_DEFAULT_\"+k.upper()+\"_MODEL\"] "
                              "for k in (\"opus\",\"sonnet\",\"haiku\")}))'"))
    check(args.auth_lab, "principal Opus", defaults["opus"], {defaults["opus"]},
          "Reply exactly TAXIWAY_MODEL_OK.")
    check(args.auth_lab, "Sonnet alias", "sonnet", {defaults["sonnet"]},
          "Reply exactly TAXIWAY_MODEL_OK.")
    check(args.auth_lab, "Haiku full ID", defaults["haiku"], {defaults["haiku"]},
          "Reply exactly TAXIWAY_MODEL_OK.")
    nested = check(args.auth_lab, "inherited subagent", defaults["opus"], {defaults["opus"]},
                   "You must invoke the inherit-worker agent once and wait for its result. "
                   "Then reply exactly TAXIWAY_MODEL_OK.", {"inherit-worker": worker("inherit")})
    assert defaults["opus"] in nested, "Inherited subagent used a different model"
    nested = check(args.auth_lab, "Sonnet and Haiku subagents", defaults["opus"], set(defaults.values()),
                   "You must invoke both sonnet-worker and haiku-worker agents, "
                   "in parallel, and wait for both results. Then reply exactly TAXIWAY_MODEL_OK.",
                   {"sonnet-worker": worker("sonnet"), "haiku-worker": worker("haiku")})
    assert {defaults["sonnet"], defaults["haiku"]} <= nested, "Subagent models were not confirmed"
    target = "models-live-" + uuid.uuid4().hex[:8]
    try:
        command([args.taxiway, "up", target, "--driver", "docker", "--type", "claude-code",
                 "--set", "model=" + defaults["sonnet"], "--skip-auth-check"], timeout=900)
        # Keep OAuth bytes only in memory and guest files (0600), never command arguments.
        credential = guest(args.auth_lab, 'cat "$HOME/.claude/.credentials.json"')
        guest(target, 'umask 077; mkdir -p "$HOME/.claude"; '
              'cat > "$HOME/.claude/.credentials.json"', data=credential)
        del credential
        command([args.taxiway, "start", target])
        check(target, "auth propagation and restarted lab", defaults["sonnet"], {defaults["sonnet"]},
              "Reply exactly TAXIWAY_MODEL_OK.")
    finally:
        command([args.taxiway, "rm", target, "--yes"], timeout=180)
    print("All 6 live Claude cases passed. Reference lab preserved.")


if __name__ == "__main__":
    try:
        main()
    except (AssertionError, RuntimeError, subprocess.TimeoutExpired) as error:
        # TimeoutExpired includes argv; never include guest shell text in diagnostics.
        print("FAIL: timeout" if isinstance(error, subprocess.TimeoutExpired) else f"FAIL: {error}", file=sys.stderr)
        sys.exit(1)
