# Gas City

The `gascity` adapter provisions [Gas City](https://gascity.com)
([gastownhall/gascity](https://github.com/gastownhall/gascity)), the
orchestration toolkit that runs Gas Town-style agent cities, with a workspace
shell. The adapter provisions Claude Code as the agent CLI inside the lab.

## Quick Start

```bash
taxiway up mylab --type gascity --repo https://github.com/org/repo
taxiway shell mylab
```

Workspace repository access is resolved by Git on the host before the lab uses a
local bare mirror. Every Gas City agent runs Claude Code through the per-lab
LiteLLM endpoint generated during `taxiway up`. Run `taxiway observe up`
separately when you also want Langfuse traces.

## Lab Layout

| Path | Contents |
|---|---|
| `/lab/work/gc` | The city (`city.toml`, `pack.toml`, `.gc/` runtime, agent directories under `.gc/agents/`) |
| `/lab/work/<repo>` | The workspace repository, registered as a rig named after the repository |
| `/lab/work/gc/taxiway.toml` | Taxiway-managed config fragment, included from `city.toml` |

During the workspace phase, Taxiway clones the repository, creates the city
when it is missing and registers the repository as a rig:

```bash
gc init /lab/work/gc --template <template> --default-provider claude \
  --providers claude --yes --no-start --skip-provider-readiness
gc rig add /lab/work/<repo> --name <repo>
```

Both steps are idempotent: an existing city keeps its configuration (the
`template` setting only applies to a new city) and a registered rig is reused.
A rig with the same name but another path fails explicitly. Gas City rigs are
whole repositories, so `--repo-path` is ignored with a warning. Without
`--repo`, the start phase creates the city without a rig.

`gc init` fetches the selected template's pack from GitHub, pinned by commit, so
the lab needs network access to GitHub when the city is created.

## Gateway Routing

Taxiway does not edit Gas City's own configuration beyond one
`include = ["taxiway.toml"]` entry in `city.toml`. On every `taxiway start`, it
rewrites the private (`0600`) `taxiway.toml` fragment:

- `[upstreams.taxiway-litellm]` points `ANTHROPIC_BASE_URL` at the lab gateway
  and sends the gateway key in `ANTHROPIC_CUSTOM_HEADERS`;
- `[agent_defaults] upstream = "taxiway-litellm"` selects it for every agent;
- `[providers.claude]` passes `--model <model>` and the managed Claude settings
  (tool search, connector import, model aliases, the gateway model list and the
  pinned-version update policy).

The fragment references the key as `$TAXIWAY_LITELLM_API_KEY`; Gas City expands
it from the supervisor environment when it launches a session, so the key is not
written into city configuration. Rewriting the whole fragment keeps restarts
idempotent and applies to every session Gas City creates later, including
restarts (`gc session reset`) and new sessions (`gc session new`).

In `auth_mode=api-key`, the upstream also renders the gateway key as the native
`ANTHROPIC_AUTH_TOKEN` credential. Subscription mode omits it, clears any
inherited token before the supervisor starts, and uses the saved native OAuth
login, as with the other Claude Code adapters.

## Supervisor and Startup

`taxiway start` stops a running city and the machine-wide Gas City supervisor,
then starts them again with `gc start`, so the supervisor carries the current
lab environment and fragment. It then waits (bounded, 240 seconds by default)
until the city reports active agent sessions and none is still being created,
and opens the `gascity` tmux session used by `taxiway shell` and
`taxiway record`.

Docker labs have no systemd user manager, so `gc start` runs the supervisor as
a detached `gc supervisor run` process that inherits the start environment. On
a Lima lab with a systemd user manager, Gas City installs a user unit instead
and only copies allow-listed variables into it. Taxiway sets
`GC_SUPERVISOR_ENV="TAXIWAY_LITELLM_API_KEY TAXIWAY_LITELLM_BASE_URL"` during
`gc start` so the fragment references resolve in both modes; in the systemd
case, Gas City writes those values into its user unit file inside the lab.

Agent sessions run on the city's own tmux socket (`tmux -L gc`), with names
such as `gastown__mayor`, `gastown__deacon` and `<rig>--gastown__witness`.

## Workspace Trust and Onboarding

Gas City launches `claude --dangerously-skip-permissions --settings
/lab/work/gc/.gc/settings.json`; that settings file already skips the bypass
warning. Taxiway trusts the city root and the rig in `~/.claude.json`, which
covers every agent directory under `.gc/agents/` and the rig, including
directories created later.

Gas City does not dismiss Claude Code's first-run screens. Complete the native
onboarding once (or copy an existing login, see
[Claude Code](claude-code.md)) before relying on agents.

## Settings

The adapter exposes these settings through `--set`:

| Setting | Description | Default |
|---|---|---|
| `version` | Exact Gas City release, verified against its published checksums | `1.5.0` |
| `beads-version` | [Beads](https://github.com/gastownhall/beads) (`bd`) release override; omitted uses the Gas City compatibility matrix | Adapter default (`1.3.1`) |
| `dolt-version` | [Dolt](https://github.com/dolthub/dolt) release override; omitted uses the Gas City compatibility matrix | Adapter default (`2.2.0`) |
| `template` | Template for a new city: `gastown`, `minimal`, or another `gc init` template | `gastown` |
| `claude-code-version` | Claude Code CLI release used by Gas City agents: `latest` or an exact release; distinct from `version` | `latest` |
| `model` | Claude Code model name passed with `--model` to every agent through LiteLLM | `claude-opus-5-5` |
| `tool-search` | Load MCP tool definitions on demand: `true`, `false`, `auto`, or `auto:N` | `true` |
| `claudeai-mcp-servers` | Import Claude.ai connectors: `true` or `false` | `false` |

Releases are exact, so a lab is reproducible: `latest` is not accepted for
`version`. The compatibility matrix follows Gas City's own CI pins; a Gas City
release without a mapping requires both `beads-version` and `dolt-version`.
Gas City and Beads archives are verified against their published checksums, and
the default Dolt archive against a pinned digest.

Settings persist with the lab; `--clear-set` restores the default. Gateway,
model and MCP settings apply at the next `taxiway start`, which restarts the
city.

Example:

```bash
taxiway up mylab --type gascity --repo https://github.com/org/repo \
  --set template=minimal --set model=claude-sonnet-5-5
```

## Shell Behavior

`taxiway shell <lab>` attaches to the `gascity` tmux session, opened in the rig
directory (or the city without `--repo`). From this shell:

| Command | Description |
|---|---|
| `gc status` | City-wide status |
| `gc session list` | Agent sessions and their state |
| `gc session attach gastown.mayor` | Join the Mayor session |
| `gc session submit gastown.mayor "<message>"` | Send a message to a session |
| `Ctrl-b d` | Detach and return to the shell while preserving the session |

On Lima, Taxiway forwards the supervisor dashboard and API (guest port `8372`)
to a per-lab host port and prints it when the city starts. Docker labs do not
publish it.

## Reset

`taxiway reset` stops the city, the supervisor and the remaining
`gc nudge poll` processes before clearing `/lab/work`.

## Limitations

- Gas City 1.5.0 drops the builtin Claude upstream binding for providers based
  on `builtin:claude`, so the fragment names `base_url_env` and
  `auth_token_env` explicitly. Remove this workaround once a release containing
  [gastownhall/gascity#5985](https://github.com/gastownhall/gascity/pull/5985)
  is pinned.
- A full `gastown` city uses about 2.2 GiB of memory and more than 200
  processes; size the lab accordingly. Starting or restarting the city takes
  one to two minutes.
- Docker labs run `sleep infinity` as PID 1, which does not reap orphaned
  children: defunct `dolt`, `bd` and `gc` processes accumulate until the lab
  restarts. They hold no memory or files.
- Gas City's Claude provider defaults to `--effort max`; Taxiway does not
  change it.
- Beads enables anonymous usage metrics by default; run `bd metrics off` in the
  lab to disable them.

## Inspect the Adapter

```bash
taxiway describe gascity
```

## Autonomous permissions

Gas City launches every agent with `--dangerously-skip-permissions`; this
adapter does not offer a restrictive permission mode.

See [guest capabilities](../contributing/live-tests.md#guest-capabilities).
