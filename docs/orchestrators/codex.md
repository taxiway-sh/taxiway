# Codex

The `codex` adapter installs and runs the OpenAI Codex CLI directly inside a
Taxiway lab.

## Quick Start

```bash
taxiway up mylab --type codex --repo https://github.com/org/repo
taxiway shell mylab
```

`taxiway shell mylab` attaches to the Codex tmux session. Send prompts directly
in that session.

The recommended lab path is to route Codex through the lab's LiteLLM sidecar.
`taxiway up` creates the lab LiteLLM key, sidecar, and proxy route. The adapter
writes the `taxiway-litellm` provider and default `gpt-6.1-sol` model into Codex's
local config during `start`. Run `taxiway observe up` separately when you also
want Langfuse traces.

Subscription authentication with Codex Pro uses host Codex OAuth converted for
LiteLLM. Run the host login once, then let Taxiway prepare the LiteLLM auth
file:

```bash
codex login
taxiway credentials codex
```

The host `~/.codex/auth.json` file is not copied into labs. Codex labs only
receive `TAXIWAY_LITELLM_API_KEY` and talk to LiteLLM.

## Settings

The adapter exposes this setting through `--set`:

| Setting | Description | Default |
|---|---|---|
| `model` | Codex model name passed through LiteLLM | `gpt-6.1-sol` |

The model name should match a Codex model name declared in LiteLLM, such as
`gpt-6.1-sol`, `gpt-6-astra`, or `gpt-6-luna`.

```bash
taxiway start mylab --set model=gpt-6-luna
taxiway start mylab --clear-set model
```

Settings persist with the lab; `--clear-set` restores the default.
`taxiway start` restarts the Codex session with the updated settings.

## Agent CLI

The adapter uses the `codex` agent, which installs the npm package
`@openai/codex`. The install phase also ensures the `bubblewrap` Linux sandboxing
utility is available, which Codex uses for process isolation.

The verify phase checks `codex --version`, `codex --help`, and whether API-key or
OAuth credentials are available, without making an API call.

## Inspect the Adapter

```bash
taxiway describe codex
```
