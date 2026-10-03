# Testing model selection and subagents

These suites use the shared `taxiway_live.py` helpers. For authentication reuse,
temporary labs and scenarios for other features, see [reusable live tests](live-tests.md).

## Automated checks without provider calls

From the repository or development worktree:

```bash
go test ./...
make lint test-scripts build
go test -tags=e2e -count=1 -timeout=180s -run '^TestE2E_Gateway' ./internal/cli
```

The last command requires Docker. It runs the shipped LiteLLM image against a
local fake provider with external networking disabled and fake credentials.
It checks Anthropic subscription/API-key routing, signed thinking replay,
ChatGPT Responses streaming, encrypted reasoning, tool replay, explicit
`parallel_tool_calls` values and provider errors. These checks do not prove that
a real account has access to a model.

## Live Claude tests with one authentication

Use a dev worktree with its `.envrc` loaded. Build the local binary so its code
matches the mounted assets. The commands below preserve the reference lab:

```bash
go build -o taxiway ./cmd/taxiway
direnv exec . ./taxiway init
direnv exec . ./taxiway up models-claude-test --driver docker --type claude-code --skip-auth-check
direnv exec . ./taxiway auth models-claude-test claude-code
direnv exec . python3 tests/live/test_claude_models.py --auth-lab models-claude-test
```

If the reference lab already exists, its persisted driver is used. Both Docker
and Lima reference labs are supported. Authenticate once in that lab; no token
needs to be pasted into a terminal command or stored in the repository. The
suite copies OAuth credentials through process memory to a temporary Docker
lab, with file permissions `0600`, and removes that temporary lab afterward.
The authenticated reference lab remains available for reruns and manual tests.

The six live cases check:

1. A principal using the configured Opus model.
2. A principal selecting the `sonnet` alias.
3. A principal selecting the full Haiku model ID.
4. A subagent inheriting the principal's model.
5. An Opus principal delegating to Sonnet and Haiku subagents.
6. Authentication propagation and a successful call after restarting a second lab.

The suite asserts client success, the answer marker, actual Agent tool calls
and forwarded subagent model IDs. It limits the tools available to the test
agents and bounds each invocation with a timeout and a client budget.

## Live Codex tests

The gateway uses the existing host Codex subscription login. With that login
available, create a reference lab and run:

```bash
direnv exec . ./taxiway up models-codex-test --driver docker --type codex
direnv exec . python3 tests/live/test_codex_models.py --lab models-codex-test
```

The four live cases check the configured principal, an alternate Luna
principal, an inherited subagent, and two subagents using Luna and Sol.
They assert successful client responses, parent/child session relationships,
the child models and provider, and actual gateway requests with output tokens.
Codex V2 may share a telemetry session ID between a principal and its children:
session counts alone are not proof of delegation. Client session journals stay
inside the reference lab. Each invocation has a timeout and a read-only sandbox.

Both live suites use real subscription requests and consume account usage.
They are opt-in and are not part of ordinary unit tests or unattended CI.
They test routing and delegation, not every catalog model, model quality, or
the accuracy of LiteLLM pricing/context metadata.

## Manual checks

```bash
direnv exec . ./taxiway describe claude-code
direnv exec . ./taxiway describe codex
direnv exec . ./taxiway shell models-claude-test
```

In Claude, try `/model sonnet`, ask a short question, then return to `/model opus`.
For explicit subagent models, define custom agents with `model: sonnet` and
`model: haiku` in the lab's `.claude/agents/` directory, restart the session,
and ask the principal to delegate one task to each. See the official
[Claude subagent documentation](https://code.claude.com/docs/en/sub-agents).

Attach to Codex with `taxiway shell models-codex-test` and ask it to spawn two
fresh-context subagents using `gpt-6-luna` and `gpt-6-sol`, wait for both, and
summarize their results. See the official
[Codex subagent documentation](https://learn.chatgpt.com/docs/agent-configuration/subagents).
Inspect generations in Langfuse to verify the models and output tokens;
an agent's assertion that it delegated is not sufficient evidence.

Remove only the reference labs when finished:

```bash
direnv exec . ./taxiway rm models-claude-test --yes
direnv exec . ./taxiway rm models-codex-test --yes
```
