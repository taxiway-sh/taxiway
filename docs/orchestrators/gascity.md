# Gas City

The `gascity` adapter provisions
[Gas City](https://github.com/gastownhall/gascity), the orchestration toolkit
that runs Gas Town-style agent cities, with a workspace shell. The adapter provisions Claude Code as the agent CLI inside the lab.

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

Taxiway edits only two keys of Gas City's own `city.toml`: one
`include = ["taxiway.toml"]` entry and the `effort` key of the Claude
provider's `option_defaults` (Gas City reads provider option defaults only from
`city.toml`). It writes `city.toml` atomically, keeps its permissions and every
other setting, and refuses an include list it cannot extend safely. On every
`taxiway start`, it also rewrites the private (`0600`) `taxiway.toml` fragment:

- `[upstreams.taxiway-litellm]` points `ANTHROPIC_BASE_URL` at the lab gateway
  and sends the gateway key in `ANTHROPIC_CUSTOM_HEADERS`;
- `[agent_defaults] upstream = "taxiway-litellm"` selects it for every agent;
- `[providers.claude]` passes `--model <model>` and the managed Claude settings
  (tool search, connector import, model aliases, the gateway model list and the
  pinned-version update policy).

The fragment references the key as `$TAXIWAY_LITELLM_API_KEY`; Gas City expands
it from the supervisor environment when it launches a session, so the key is not
written into city configuration (see [Gateway key exposure](#gateway-key-exposure)
for where it does appear). Rewriting the whole fragment keeps restarts
idempotent and applies to every session Gas City creates later, including
restarts (`gc session reset`) and new sessions (`gc session new`).

In `auth_mode=api-key`, the upstream also renders the gateway key as the native
`ANTHROPIC_AUTH_TOKEN` credential. Subscription mode omits it, clears any
inherited token before the supervisor starts, and uses the saved native OAuth
login, as with the other Claude Code adapters.

## Supervisor and Startup

`taxiway start` stops a running city and the machine-wide Gas City supervisor,
then starts them again with Gas City's official method for the lab's
environment type (`TAXIWAY_GUEST_KIND`), so the supervisor carries the current
lab environment and fragment. It then waits (bounded, 240 seconds by default)
until the city reports active agent sessions and none is still being created,
and opens the `gascity` tmux session used by `taxiway shell` and
`taxiway record`.

| Lab | Guest | Supervisor |
|---|---|---|
| Docker | `container` | `gc supervisor start` runs the canonical `gc supervisor run` loop detached, as Gas City's own container images run it under an init; then `gc start` registers the city. No service unit is installed. |
| Lima | `vm` | `gc start` installs, enables and starts Gas City's systemd user unit. |

In a Docker lab the detached supervisor inherits the start environment, and
Docker's init (PID 1) reaps the processes the city leaves behind. The
container has no systemd user manager, so Gas City never installs a unit
there; `gc start` still prints that a per-user systemd instance is not
available, which is expected in a container guest (start says so just before).
If a running supervisor does not stop after one bounded retry, start fails
with the stop error instead of reusing it, in both environments. A Lima unit only receives allow-listed variables: Taxiway sets
`GC_SUPERVISOR_ENV="TAXIWAY_LITELLM_API_KEY TAXIWAY_LITELLM_BASE_URL"` during
`gc start` so the fragment references resolve. The per-environment runtime
contract is tracked in #179.

### Gateway key exposure

On a Lima lab, Gas City writes its supervisor environment into
the enabled user unit `~/.local/share/systemd/user/gascity-supervisor.service`
(mode `0600`). The unit stores `TAXIWAY_LITELLM_API_KEY`,
`TAXIWAY_LITELLM_BASE_URL` and every `ANTHROPIC_*` variable of the start
environment, including `ANTHROPIC_AUTH_TOKEN` (the gateway key) in
`auth_mode=api-key` and the `ANTHROPIC_DEFAULT_*_MODEL` aliases. The unit
persists across `taxiway reset` until the lab is removed. Docker labs have no
such unit.

On every driver, Gas City starts each session with `tmux new-session -e
NAME=value`, so the session environment, including the gateway key header and
token, briefly appears in the guest's process arguments. Keeping the gateway key
in a single guest location is tracked in #180.

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
| `template` | Template for a new city: `gastown` or `minimal` (both define always-on sessions, which start waits for) | `gastown` |
| `claude-code-version` | Claude Code CLI release used by Gas City agents: `latest` or an exact release; distinct from `version` | `latest` |
| `model` | Claude Code model name passed with `--model` to every agent through LiteLLM | `claude-opus-5-5` |
| `effort` | Claude Code effort passed with `--effort` to every agent: `low`, `medium`, `high`, `xhigh`, or `max` | `max` (Gas City's default) |
| `tool-search` | Load MCP tool definitions on demand: `true`, `false`, `auto`, or `auto:N` | `true` |
| `claudeai-mcp-servers` | Import Claude.ai connectors: `true` or `false` | `false` |

Releases are exact, so a lab is reproducible: `latest` is not accepted for
`version`. Unsupported `template` or `effort` values fail before the city is
created or changed. The compatibility matrix follows Gas City's own CI pins; a Gas City
release without a mapping requires both `beads-version` and `dolt-version`.
Gas City and Beads archives are verified against their published checksums, and
the default Dolt archive against a pinned digest.

Settings persist with the lab; `--clear-set` restores the default. Gateway,
model, effort and MCP settings apply at the next `taxiway start`, which
restarts the city.

The install phase also disables Beads anonymous usage metrics
(`bd metrics off`, stored in `~/.config/bd`) in Gas City labs.

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
| `gc suspend` / `gc resume` | Pause the city between uses, then resume it; see [Pause the City](#pause-the-city) |
| `Ctrl-b d` | Detach and return to the shell while preserving the session |

On Lima, Taxiway forwards the supervisor dashboard and API (guest port `8372`)
to a per-lab host port and prints it when the city starts. Docker labs do not
publish it.

## Pause the City

An idle `gastown` city keeps calling the model (see
[Limitations](#limitations)). Pause it whenever it is not in use, from
`taxiway shell <lab>`. When you stop using the lab:

```bash
cd /lab/work/gc && gc suspend
```

Before you use it again, or before a Taxiway command restarts the city:

```bash
cd /lab/work/gc && gc resume
```

`gc suspend` stops the agent sessions, and the city stays suspended until
`gc resume`, even when the lab is stopped and started again. Resume it before
you stop the lab: Taxiway's start phase waits for active agent sessions, so
restarting a suspended city times out. If that happens, `taxiway start --force`
recreates the city, without its state; `taxiway reset` also deletes the city.

## Reset

`taxiway reset` runs the adapter's `stop.sh`, which stops the city, the
supervisor and the remaining `gc nudge poll` processes, before clearing
`/lab/work`. If the supervisor does not stop after one bounded retry, reset
fails without deleting anything.

## Limitations

- Gas City 1.5.0 drops the builtin Claude upstream binding for providers based
  on `builtin:claude`, so the fragment names `base_url_env` and
  `auth_token_env` explicitly. Removing this workaround once a release
  containing [gastownhall/gascity#5985](https://github.com/gastownhall/gascity/pull/5985)
  is pinned is tracked in #178.
- On Lima, the gateway key is also stored in Gas City's supervisor unit, and
  sessions receive it through `tmux -e` arguments; see
  [Gateway key exposure](#gateway-key-exposure) (tracked in #180).
- An idle `gastown` city keeps calling the model. Gas City 1.5.0 pins the Gas
  Town pack at commit `33d3a43`, whose always-on patrol agents restart instead
  of backing off and reload their full prompt on each fresh start
  ([gastownhall/gascity#5727](https://github.com/gastownhall/gascity/issues/5727),
  [gastownhall/gascity#4911](https://github.com/gastownhall/gascity/issues/4911),
  [gastownhall/gascity#1751](https://github.com/gastownhall/gascity/issues/1751),
  [gastownhall/gascity-packs#268](https://github.com/gastownhall/gascity-packs/issues/268));
  the boot fix [gastownhall/gascity-packs#261](https://github.com/gastownhall/gascity-packs/pull/261)
  and its follow-up [gastownhall/gascity-packs#420](https://github.com/gastownhall/gascity-packs/pull/420)
  are not in that pack. On an idle Lima lab with the default model and
  `effort=high`, boot started a fresh session about every 30 seconds and the
  witness ran back-to-back patrol cycles, for about $40 of gateway spend per
  hour. Spend depends on the model and effort; the default effort is `max`
  ([Settings](#settings)). [Pause the City](#pause-the-city) when it is not
  in use; tracked in #259.
- Every `gc` command prints a `named_session ... mode "always" with wake_mode
  "fresh"` warning for the mayor, deacon and boot, and for each rig's
  witness. The pack chooses this lifecycle on purpose and Gas City cannot
  acknowledge it yet
  ([gastownhall/gascity#7674](https://github.com/gastownhall/gascity/issues/7674));
  tracked in #259.
- A full `gastown` city uses about 2.2 GiB of memory and more than 200
  processes; size the lab accordingly. Starting or restarting the city takes
  one to two minutes.
- Docker labs created before Taxiway added `--init` keep `sleep infinity` as
  PID 1, which does not reap Gas City's orphaned `dolt`, `bd` and `gc`
  processes; recreate them.

## Inspect the Adapter

```bash
taxiway describe gascity
```

## Autonomous permissions

Gas City launches every agent with `--dangerously-skip-permissions`; this
adapter does not offer a restrictive permission mode.

See [guest capabilities](../contributing/live-tests.md#guest-capabilities).
