# Model gateway validation

Durable gateway and runtime regressions belong in E2Es. Agents validate real
account/model behavior as temporary manual checks using the shared tooling in
[Live validation](live-tests.md). Real-provider requests are explicitly opt-in;
choose only the checks relevant to the feature, with short prompts, command
bounds and client budgets where supported. Keep scripts and reports outside Git.

## Automated checks without provider calls

```bash
go test ./...
make lint test-scripts build
go test -tags=e2e -count=1 -timeout=180s -run '^TestE2E_Gateway' ./internal/cli
```

The gateway E2Es use the shipped LiteLLM image, a local controlled provider,
fake credentials and disabled external networking. They cover Anthropic
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
