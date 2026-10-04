"""Opt-in live Codex tests against an existing Taxiway dev/e2e lab."""

import argparse
from datetime import datetime, timezone
import json
import shlex
import subprocess
import sys
import time

from taxiway_live import command, guest, run_in_lab, runtime_id, temporary_lab, validate_context


def metrics(lab, since):
    runtime = runtime_id(lab)
    # Only models, token counts and counts of distinct sessions leave the DB.
    rows = ('FROM "LiteLLM_SpendLogs" WHERE "startTime" >= '
            f"TIMESTAMP '{since}' AND completion_tokens > 0 ")
    query = ('SELECT model, count(DISTINCT session_id), sum(completion_tokens) '
             + rows + "GROUP BY model UNION ALL SELECT '__sessions__', "
             "count(DISTINCT session_id), sum(completion_tokens) " + rows)
    output = command(["docker", "exec", runtime + "-gateway-postgres-1",
                      "psql", "-U", "litellm", "-d", "litellm", "-At", "-c", query])
    result = {name.removeprefix("chatgpt/"): (int(sessions), int(tokens))
              for name, sessions, tokens in (line.split("|") for line in output.decode().splitlines())}
    sessions = result.pop("__sessions__", (0, 0))[0]
    return result, sessions


def child_models(lab, parent):
    script = "python3 - " + shlex.quote(parent) + " <<'PY'\n" + '''
from pathlib import Path
import json, sys
children = []
for path in (Path.home()/".codex/sessions").rglob("*.jsonl"):
    meta, model, context = {}, None, {}
    for line in path.read_text().splitlines():
        try:
            entry = json.loads(line)
        except ValueError:
            continue
        if entry.get("type") == "session_meta":
            meta = entry["payload"]
        elif entry.get("type") == "turn_context":
            context = entry.get("payload", {})
            model = context.get("model")

    if meta.get("parent_thread_id") == sys.argv[1]:
        assert context.get("approval_policy") == "never", "child lost approval bypass"
        assert context.get("sandbox_policy", {}).get("type") == "danger-full-access", "child retained sandbox"
        children.append({"model": model, "provider": meta.get("model_provider")})
print(json.dumps(children))
PY'''
    return json.loads(guest(lab, script))


def check(lab, label, model, prompt, expected, *, expected_children=None):
    agents = expected_children is not None
    since = datetime.now(timezone.utc).strftime("%Y-%m-%d %H:%M:%S.%f")
    argv = ["codex", "exec", "--skip-git-repo-check", "--json",
            "-m", model, "-c", 'model_reasoning_effort="low"',
            "-c", "features.multi_agent_v2=true" if agents else "agents.enabled=false", prompt]
    output = run_in_lab(lab, argv, agent="codex")
    events = [json.loads(line) for line in output.splitlines() if line.startswith(b"{")]
    assert any(e.get("type") == "turn.completed" for e in events), "Turn did not complete"
    assert not any(e.get("type") in ("error", "turn.failed") for e in events), "Client reported an error"
    assert any("TAXIWAY_MODEL_OK" in e.get("item", {}).get("text", "") for e in events), "Expected answer marker missing"
    parent = next(e["thread_id"] for e in events if e.get("type") == "thread.started")
    observed = {}
    # Spend logs are written asynchronously. Do not accept an answer alone as evidence.
    for _ in range(20):
        observed, sessions = metrics(lab, since)
        if expected <= observed.keys():
            break
        time.sleep(1)
    assert expected <= observed.keys(), f"Expected models {sorted(expected)}; observed {sorted(observed)}"
    children = child_models(lab, parent)
    if agents:
        assert len(children) == len(expected_children), "Unexpected number of subagent sessions"
        assert expected_children <= {c["model"] for c in children}, "Subagent session models not confirmed"
        assert all(c["provider"] == "taxiway-litellm" for c in children), "A subagent bypassed the gateway"
    else:
        assert not children, "Unexpected subagent session"
    print(f"PASS {label}: models={','.join(sorted(observed))}; subagents={len(children)}; sessions={sessions}", flush=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--lab", required=True)
    args = parser.parse_args()
    validate_context()
    main_model = guest(args.lab, "python3 -c 'import pathlib,tomllib; "
                       "print(tomllib.loads((pathlib.Path.home()/\".codex/config.toml\").read_text())[\"model\"])'").decode().strip()
    with temporary_lab("codex", settings={"model": main_model}) as target:
        check(target, "principal", main_model,
              "Reply exactly TAXIWAY_MODEL_OK. Do not use any tools.", {main_model})
        check(target, "alternate principal", "gpt-6-luna",
              "Reply exactly TAXIWAY_MODEL_OK. Do not use any tools.", {"gpt-6-luna"})
        check(target, "inherited subagent", main_model,
              "Spawn exactly one subagent with fresh context. Do not specify its model or reasoning: "
              "it must inherit yours. Ask it to reply exactly TAXIWAY_MODEL_OK without tools. "
              "Wait for its completion and then reply exactly TAXIWAY_MODEL_OK. "
              "Do not use shell, browser, file, or MCP tools.", {main_model}, expected_children={main_model})
        check(target, "two different subagent models", main_model,
              "Spawn exactly two subagents with fresh context, one using gpt-6-luna "
              "and one using gpt-6-sol, both with low reasoning effort. Explicitly choose "
              "fork_turns none or fork_context false when spawning. Each must reply exactly "
              "TAXIWAY_MODEL_OK without tools. Wait for both to finish, then reply exactly "
              "TAXIWAY_MODEL_OK. Do not use shell, browser, file, or MCP tools.",
              {main_model, "gpt-6-luna", "gpt-6-sol"}, expected_children={"gpt-6-luna", "gpt-6-sol"})
        command(["./taxiway", "start", target])
        prompt = "Spawn exactly one subagent with fresh context inheriting your model. Ask it to use shell to write print(42) followed by a newline to /lab/work/autonomous-proof.py, run python3 -m py_compile on it, and run curl --max-time 20 -fsSI https://example.com > /lab/work/network-proof. Wait for completion, then use shell to cat the proof file and reply TAXIWAY_MODEL_OK."
        check(target, "autonomous restarted lab and delegated tools", main_model, prompt, {main_model}, expected_children={main_model})
        assert guest(target, "cat /lab/work/autonomous-proof.py").strip() == b"print(42)", "Actual child edit missing"
        assert guest(target, "test -s /lab/work/network-proof && find /lab/work/__pycache__ -name 'autonomous-proof*' -print -quit").strip(), "Network/build effects missing"
    print("All live Codex cases passed. Reference lab preserved.")


if __name__ == "__main__":
    try:
        main()
    except (AssertionError, RuntimeError, subprocess.TimeoutExpired) as error:
        print("FAIL: timeout" if isinstance(error, subprocess.TimeoutExpired) else f"FAIL: {error}", file=sys.stderr)
        sys.exit(1)
