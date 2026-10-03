"""Opt-in live Claude tests. Run with an authenticated reference lab.

Uses real subscription requests; never prints or stores OAuth credentials.
The reference lab is preserved. A second, temporary Docker lab is removed.
"""

import argparse
import json
import subprocess
import sys

from taxiway_live import command, guest, require_claude_auth, run_in_lab, temporary_lab, validate_context


def run_claude(lab, model, prompt, agents=None):
    argv = ["claude", "-p", prompt, "--model", model,
            "--output-format", "stream-json", "--verbose",
            "--forward-subagent-text", "--no-session-persistence",
            "--effort", "low", "--permission-mode", "dontAsk",
            "--tools", "Agent" if agents else "",
            "--allowedTools", "Agent", "--max-budget-usd", "2"]
    if agents:
        argv += ["--agents", json.dumps(agents)]
    output = run_in_lab(lab, ["env", "-u", "CLAUDE_CODE_SUBAGENT_MODEL",
                              "-u", "CLAUDE_CODE_SUBAGENT_MODEL_FORCE",
                              "CLAUDE_AGENT_SDK_DISABLE_BUILTIN_AGENTS=1", *argv],
                        agent="claude-code")
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
    validate_context()
    require_claude_auth(args.auth_lab)
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
    with temporary_lab("claude-code", auth_lab=args.auth_lab,
                       settings={"model": defaults["sonnet"]}, taxiway=args.taxiway) as target:
        command([args.taxiway, "start", target])
        check(target, "auth propagation and restarted lab", defaults["sonnet"], {defaults["sonnet"]},
              "Reply exactly TAXIWAY_MODEL_OK.")
    print("All 6 live Claude cases passed. Reference lab preserved.")


if __name__ == "__main__":
    try:
        main()
    except (AssertionError, RuntimeError, subprocess.TimeoutExpired) as error:
        # TimeoutExpired includes argv; never include guest shell text in diagnostics.
        print("FAIL: timeout" if isinstance(error, subprocess.TimeoutExpired) else f"FAIL: {error}", file=sys.stderr)
        sys.exit(1)
