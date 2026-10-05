# Model gateway validation

Gateway assertions protect each orchestrator's lifecycle: configured, started,
routed and restarted. They enrich the nine existing scenarios and
helpers in `internal/cli/orchestrator_e2e_test.go`. Do not add a standalone
protocol runner, another E2E suite/file, entry point, job or Makefile runner. Agents validate real
account/model behavior as temporary manual checks using the shared tooling in
[Live validation](live-tests.md). Real-provider requests are explicitly opt-in;
choose only the checks relevant to the feature, with short prompts, command
bounds and client budgets where supported. Keep scripts and reports outside Git.

## Automated checks without provider calls

```bash
go test ./...
make lint test-scripts build
make test-e2e-claude-code-up
make test-e2e-codex-up
```

Existing lifecycle scenarios exercise their actual lab LiteLLM gateway with
a local controlled provider and fake credentials. They cover Anthropic
subscription/API-key routing, signed thinking replay, Responses streaming,
encrypted reasoning, tool replay, `parallel_tool_calls` and provider errors.
Orchestrator E2Es cover shipped model defaults/selections, configuration,
interactive completion and restart/handoff propagation. These checks cannot
establish real account access or model-driven delegation.

## Choose a real-account check

Use existing labs `test-claude`, `test-codex` and `test-gastown` in the isolated
worktree context, or their actual existing names. Follow the live guide for
reference preflight, authentication propagation and owned temporary labs. Codex
uses the existing host login through its gateway; do not copy auth into clients.

For Claude model selection, try the configured principal model, an alias such
as `sonnet`, or a full catalog ID relevant to the change. For delegation, define
a small custom agent with an explicit model or inherited model and ask the
principal to invoke it once and wait for its result. Capture bounded
`stream-json` output with `--verbose --forward-subagent-text`; verify client
success, the actual Agent tool invocation and child response model IDs.
Use `run_in_lab` to load the target's own gateway environment. After a restart,
repeat a bounded real request to establish auth and routing still work; credential
file presence alone is insufficient.

For Codex, ask for one fresh-context subagent with the model needed for the
feature, then wait for completion. Inspect guest session journals for actual
parent/child relationships, exact expected child count, requested child models, the `taxiway-litellm`
provider and inherited approval/sandbox settings. Inspect matching gateway
requests and positive output tokens. A shared telemetry session ID or session
count alone does not prove delegation. Gateway/Langfuse evidence must identify
observed models, not just the model requested in a prompt.

For permission/tool features, ask for a harmless proof file and a tiny build
such as `python3 -m py_compile`; inspect the file, compiled artifact, tool events
and absence of permission denials. If outbound networking is affected, use one
bounded HTTPS request and inspect its result. Test delegated actions when the
change affects children, without supplying extra permission bypass flags that
could conceal a broken shipped default.

For Gas Town startup/patrol changes, inspect `gt status --json`, actual role
processes, onboarding readiness and the selected model/gateway environment.
Observe Deacon tool results for at least three distinct patrol checks, a
successful `gt patrol report` and advancing heartbeat cycle. An alive daemon or
heartbeat alone is insufficient. If handoff is affected, check a replaced
Deacon/Mayor process and fresh patrol progress after `gt handoff`; a self-sling
interruption is a failure even if the daemon recovers. A separate fresh lab can
expose startup failures hidden by populated hooks. Stop background patrols once
the selected observations are complete to bound account usage. This does not
qualify a full polecat/refinery/merge-queue workload.

For Gas Town inference, execute a bounded request through its shipped
`launch-agent.sh`, then inspect principal/child model responses and actual file,
build or network effects relevant to the feature. Do not inject replacement
gateway settings that bypass the launcher being validated.

For interactive changes, attach with `taxiway shell <lab>` and verify real
completion without onboarding/authentication/permission loops. Headless success
alone does not establish interactive readiness.

Record the tested commit, driver, client versions, selected checks, actual
effects and sanitized evidence. Mark blocked or unexecuted paths explicitly.
Preserve reference labs and clean only owned temporary resources. See
[Claude subagents](https://code.claude.com/docs/en/sub-agents) and
[Codex subagents](https://learn.chatgpt.com/docs/agent-configuration/subagents)
for client usage.

## Evidence collection examples

Use these fragments inside a temporary feature check, choosing `driver="docker"`
or `driver="lima"` in `temporary_lab` according to the feature. The shared helper
resolves guest execution from the persisted driver; gateway containers still use
Docker for both. Capture output locally and return only relevant evidence.

For a bounded Claude observation, choose the model/agents relevant to the task:

```python
output = run_in_lab(lab, ["claude", "-p", prompt, "--model", selected_model,
    "--output-format", "stream-json", "--verbose", "--forward-subagent-text",
    "--no-session-persistence", "--effort", "low", "--tools", "Agent,Bash",
    "--max-budget-usd", "1", "--agents", json.dumps(selected_agents)],
    agent="claude-code", timeout=180)
events = [json.loads(line) for line in output.splitlines() if line.startswith(b"{")]
results = [event for event in events if event.get("type") == "result"]
assert results and results[-1].get("subtype") == "success"
assert not results[-1].get("is_error") and not results[-1].get("permission_denials")
children = {event.get("message", {}).get("model") for event in events
            if event.get("type") == "assistant" and event.get("parent_tool_use_id")}
agent_calls = [block.get("input", {}).get("subagent_type") for event in events
    if event.get("type") == "assistant"
    for block in event.get("message", {}).get("content", [])
    if block.get("type") == "tool_use" and block.get("name") == "Agent"]
# Assert the requested child identities/models, then inspect their actual effects.
```

For Codex, use `run_in_lab(..., agent="codex", timeout=180)` with
`codex exec --skip-git-repo-check --json -m <model> -c
'features.multi_agent_v2=true' <short-prompt>`. Require `turn.completed`, no
`error`/`turn.failed`, and extract `thread_id` from `thread.started`. A bounded
guest Python query over `~/.codex/sessions/**/*.jsonl` reads only these fields:

```python
# Inside the guest; parent is the thread ID from the current client observation.
children = []
for path in (Path.home() / ".codex/sessions").rglob("*.jsonl"):
    meta, context = {}, {}
    for line in path.read_text().splitlines():
        try: event = json.loads(line)
        except ValueError: continue
        if event.get("type") == "session_meta": meta = event.get("payload", {})
        if event.get("type") == "turn_context": context = event.get("payload", {})
    if meta.get("parent_thread_id") == parent:
        children.append({"model": context.get("model"),
                         "provider": meta.get("model_provider"),
                         "approval": context.get("approval_policy"),
                         "sandbox": context.get("sandbox_policy", {}).get("type")})
# Return only this metadata, never full journals or arbitrary message content.
```

Execute the guest query with `guest(..., timeout=30)` and assert the expected
count/models, provider `taxiway-litellm`, approval `never`, sandbox
`danger-full-access`. For tools, check exact harmless file contents and a compiled
artifact with `guest`, plus the bounded network-proof file where relevant.

For gateway evidence, identify the lab's gateway Postgres container using its
owned runtime ID. Query only aggregates after the observation's UTC start time,
not full request logs; poll for at most 20 seconds because writes are asynchronous:

```sql
SELECT model, count(DISTINCT session_id), sum(completion_tokens)
FROM "LiteLLM_SpendLogs"
WHERE "startTime" >= TIMESTAMP '<observation-start-UTC>' AND completion_tokens > 0
GROUP BY model LIMIT 20;
```

Run bounded `docker exec <owned-gateway-postgres> psql -U litellm -d litellm
-At -c <query>` through `command(..., timeout=30)`. Assert observed models and
positive output tokens. Langfuse generations can establish the same evidence;
a requested model, response marker or session count cannot replace it.

For Gas Town, read `gt status --json` and its `tmux.socket`. Use that socket for
`tmux -L <socket> display-message -p -t hq-deacon
'#{pane_pid}|#{pane_dead}|#{pane_current_command}'` before/after handoff; require a
new live Claude process. Inspect process argv/environment **inside the guest**,
returning booleans for model, gateway/alias and permission settings rather than
values containing secrets. Apply the same technique to Mayor when relevant.

For actual patrols, inspect `/lab/work/gt/deacon/heartbeat.json` and Claude journals
under `~/.claude/projects/**/*.jsonl`, filtering `cwd` to `/lab/work/gt/deacon`.
Match `tool_use` IDs for Bash to `tool_result.tool_use_id`; only successful
results count. Useful checks include `gt mail inbox`, `gt convoy`, `gt witness
status`, `gt refinery status`, `gt deacon cleanup-orphans`, `gt dog`, `gt dispatch`.
A successful `gt patrol report` must show a closed patrol and a new patrol;
heartbeat cycle and successful check/report counts must advance after the
observation's baseline. Flag any errored/interrupted result for
`gt sling mol-deacon-patrol deacon` rather than treating later recovery as success.
Return only distinct checks/counts/report count/cycle/interruption booleans.
Bound polling (for example 180 seconds for fresh startup, 600 for a complete
patrol) and guest inspections (30–90 seconds); stop `gt down` once selected
observations finish. Do not print full transcripts or unlimited role logs.
