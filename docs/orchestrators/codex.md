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

The adapter exposes these settings through `--set`:

| Setting | Description | Default |
|---|---|---|
| `codex-version` | Codex CLI release: `latest` or an exact release | `latest` |
| `model` | Codex model name passed through LiteLLM | `gpt-6.1-sol` |

The model name should match a Codex model name declared in LiteLLM, such as
`gpt-6.1-sol`, `gpt-6-astra`, or `gpt-6-luna`.

```bash
taxiway start mylab --set model=gpt-6-luna
taxiway start mylab --clear-set model
```

Settings persist with the lab; `--clear-set` restores the default.
`taxiway start` restarts the Codex session with the updated settings.

At startup, the adapter obtains the installed CLI's bundled model catalog with
`codex debug models --bundled` and configures `model_catalog_json`. It preserves
the native model capabilities and removes catalog upgrade suggestions so an
interactive migration prompt cannot replace the model selected through
Taxiway. Changing the model remains an explicit `--set model=...` operation.
This requires the native catalog command and configuration supported by Codex
`0.160.0`; an unsupported CLI fails startup instead of creating guessed metadata.

### Agent Version

`codex-version` selects the Codex CLI release installed in the lab. It is
independent of `model`, which selects the LLM.

| Value | Behavior |
|---|---|
| omitted or `latest` | Install the latest release in a fresh lab; keep whatever release is already installed afterwards. Not an immutable pin. |
| exact release, e.g. `0.160.0` | Install exactly this release of `@openai/codex`, upgrading or downgrading an existing installation. Codex runs with `check_for_update_on_startup = false` so it does not offer to replace itself. |

```bash
taxiway up mylab --type codex --set codex-version=0.160.0
taxiway install mylab --set codex-version=<other-release>
taxiway install mylab --clear-set codex-version
```

The pin persists with the lab, so stop/start, phase resumes, and reinstalls use
the same release. Changing it reinstalls the agent without touching the
workspace or credentials. Clearing it returns to `latest` and keeps the
installed release.

Only `latest` and exact releases are accepted; ranges and other npm tags are
rejected. An unknown release fails the install phase without falling back to
another release. The verify phase prints `codex --version`; for an unpinned lab,
use that release to pin the same one in another lab.

## Agent CLI

The adapter uses the `codex` agent, which installs the npm package
`@openai/codex`. The install phase also ensures the `bubblewrap` Linux sandboxing
utility is available, which Codex uses for process isolation.

The verify phase checks `codex --version`, `codex --help`, and whether the lab's
LiteLLM gateway key is configured. A missing key prints guidance; this phase
does not validate host OAuth, gateway readiness or provider access. Those need
a successful request through the gateway.

## Inspect the Adapter

```bash
taxiway describe codex
```

`taxiway list <lab>` shows the requested harness version and the last observed
installed release and executable path. Installation and `taxiway verify <lab>`
refresh that observation in the lab's `agent-versions.json` state; the requested
version remains in its saved settings. Verification fails if the executable on
the launch PATH differs from an exact pin.

## Autonomous permissions

Codex runs with Full Access inside the guest: `approval_policy = "never"` and
`sandbox_mode = "danger-full-access"`, including delegated agents. Fresh and
resumed interactive launches also explicitly pass
`--dangerously-bypass-approvals-and-sandbox` to override saved session restrictions.

See [guest capabilities](../contributing/live-tests.md#guest-capabilities).
