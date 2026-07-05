# Gateway

Taxiway's gateway path routes lab model traffic through LiteLLM. It is separate
from the optional Langfuse observability stack.

- **Gateway path:** shared Caddy proxy, one LiteLLM sidecar per lab, and one
  small gateway Postgres database per lab.
- **Observability stack:** Langfuse and its storage services for trace
  inspection.

Gateways can run without Langfuse. When Langfuse is running and the lab has
generated local Langfuse keys, LiteLLM exports traces to the observability
stack.

For the Langfuse side, see [Observability](observability.md).

## Prerequisites

- `docker` on `PATH` and a reachable Docker daemon
- a created lab

Lab gateways run on Docker even when the lab itself uses the Lima driver.

## Quick Start

Create or refresh a lab gateway:

```bash
taxiway gateway <lab>
```

`taxiway up <lab>` and `taxiway run <lab>` also run the `gateway` phase
automatically.

Print the lab endpoint and key:

```bash
taxiway access
```

Use the lab-specific hostname:

```text
http://<lab>.litellm.localhost:<proxy-port>/v1
http://<lab>.litellm.localhost:<proxy-port>/ui/login
```

When Taxiway is installed, the default proxy port is usually `4000`; run
`taxiway access` to print the actual port.

## What The Gateway Phase Creates

For each lab, the gateway phase:

- generates `TAXIWAY_LITELLM_API_KEY`;
- writes the lab gateway environment;
- creates a lab-specific LiteLLM Compose project;
- starts LiteLLM and a small gateway Postgres database;
- registers `<lab>.litellm.localhost` in the shared Caddy proxy;
- configures Langfuse export when observability has been initialized.

Lab-specific gateway files live under:

```text
~/.taxiway/lab-state/<lab>/gateway
```

The runtime LiteLLM model catalog lives under:

```text
~/.taxiway/runtime/infra/gateway/litellm/models.yaml
```

## Access And Credentials

Run:

```bash
taxiway access
```

For each running gateway, Taxiway prints:

- the LiteLLM UI URL;
- the OpenAI-compatible API URL;
- the lab gateway key.

Use `admin` as the LiteLLM UI username. Use the printed `Password/API key`
value as both the UI password and the API bearer key.

Labs authenticate to LiteLLM with `TAXIWAY_LITELLM_API_KEY`. Provider
subscription credentials stay with the user agent whenever possible; LiteLLM
receives the gateway key and, for subscription-backed routes, forwards the
client's provider auth header.

## Codex Subscription Routing

If host Codex auth exists, Taxiway can convert it into the format LiteLLM needs
for ChatGPT subscription-backed Codex models:

```bash
codex login
taxiway credentials codex
taxiway gateway <lab>
```

`taxiway credentials codex` expects an existing host Codex login at
`~/.codex/auth.json`. It does not run `codex login` for you.

For Codex subscription testing, attach to the lab session:

```bash
taxiway shell <lab>
```

Then send a small prompt in the attached Codex session:

```text
Respond with only: codex-subscription-ok
```

Codex uses a custom OpenAI-compatible provider that points to the lab LiteLLM
gateway:

```toml
model_provider = "taxiway-litellm"
model = "gpt-5.5"

[model_providers.taxiway-litellm]
name = "Taxiway LiteLLM"
base_url = "http://<lab>.litellm.localhost:4000/v1"
wire_api = "responses"
requires_openai_auth = true
env_http_headers = { "x-litellm-api-key" = "TAXIWAY_LITELLM_API_KEY", "x-litellm-agent-id" = "TAXIWAY_LITELLM_AGENT_ID" }
supports_websockets = false
```

Codex supplies the OpenAI/ChatGPT auth. `x-litellm-api-key` authenticates the
gateway call. Taxiway deliberately does not send `x-litellm-session-id` for
Codex because that header would override the native Codex session. LiteLLM maps
Codex's `x-codex-turn-metadata.session_id` header into the Langfuse session
when observability is enabled.

## Claude Code Max Routing

For Claude Code Max subscriptions, Claude Code can point Anthropic traffic at
LiteLLM and let LiteLLM forward Claude Code's OAuth `Authorization` header to
Anthropic:

```bash
export ANTHROPIC_BASE_URL=http://<lab>.litellm.localhost:4000
export ANTHROPIC_CUSTOM_HEADERS="x-litellm-api-key: Bearer $TAXIWAY_LITELLM_API_KEY"
```

Use a full Claude model name declared in the LiteLLM catalog from the attached
Claude Code session. Examples include:

```text
claude-opus-4-8
claude-sonnet-4-6
```

Claude Code keeps the user OAuth token client-side. LiteLLM receives and
forwards it. The LiteLLM config must set:

```yaml
general_settings:
  forward_client_headers_to_llm_api: true
```

Otherwise Anthropic receives neither the Claude Code OAuth `Authorization`
header nor an API key and returns an authentication error.

Taxiway deliberately does not expose Claude Code aliases such as `opus`,
`sonnet`, or `haiku` through LiteLLM. Use full model names from the catalog.

## Traces

LiteLLM exports successful calls to Langfuse when:

- `taxiway observe up` or `taxiway init` has initialized observability;
- the lab gateway has been refreshed after observability was initialized;
- model traffic flows through the lab LiteLLM gateway.

To verify traces, start observability, refresh the gateway, and attach to the
lab session:

```bash
taxiway observe up
taxiway gateway <lab>
taxiway shell <lab>
```

Then send a small prompt in the attached orchestrator session:

```text
Respond with only: trace-ok
```

Then open Langfuse:

```bash
taxiway observe open
```

If traces are missing, first check the gateway path with `taxiway access` and
the LiteLLM sidecar logs. The observability stack only stores traces after the
gateway has exported them.

## Troubleshooting

| Symptom | Check |
|---|---|
| Gateway URL unavailable | Run `taxiway status`; verify Docker, the shared proxy, and the lab gateway containers |
| LiteLLM UI auth fails | Use `admin` and the `Password/API key` printed by `taxiway access` |
| Codex route fails auth | Run `codex login`, then `taxiway credentials codex`, then `taxiway gateway <lab>` |
| Claude route fails auth | Verify Claude Code is logged in and `forward_client_headers_to_llm_api` is enabled |
| Langfuse has no traces | Start observability, refresh the gateway, then send model traffic through LiteLLM |
