# Gas Town

The `gastown` adapter provisions [gastownhall/gastown](https://gastownhall.ai)
with a workspace shell. The adapter provisions Claude Code as the agent CLI
inside the lab.

## Quick Start

```bash
taxiway up mylab --type gastown --repo https://github.com/org/repo
taxiway shell mylab
```

Workspace repository access is resolved by Git on the host before the lab uses a
local bare mirror. Gas Town runs a single Claude Code preset through the per-lab
LiteLLM endpoint generated during `taxiway up`. Run `taxiway observe up`
separately when you also want Langfuse traces.

Taxiway does not vendor or copy a full Gas Town default configuration. During
start, it copies any selected profile settings first, then patches only the
runtime routing keys needed for LiteLLM:

- `settings/config.json` sets `default_agent` and known `role_agents` to the
  Taxiway-managed Claude Code preset.
- `settings/agents.json` defines that preset with the LiteLLM base URL and
  authentication headers.

The preset launches Claude through `orchestrators/gastown/launch-agent.sh`.
Before each launch, the launcher calls the Claude workspace-trust hook to trust
the current directory in `~/.claude.json`, then replaces itself with Claude,
preserving the arguments and environment. The hook only approves a supplied
path; it does not launch agents. This also covers dynamically created workers
using the preset. The preset explicitly declares `claude` and `node` as its
`process_names` so Gas Town recognizes the final process rather than treating
the completed launcher as a dead agent.
Automatic trust is restricted to the configured Gas Town directory (normally
`/lab/work/gt`); directories outside it, including symlink escapes, are rejected.
The preset passes this root as a launcher argument so it survives `gt handoff`.
On every launch, including handoff, the launcher reloads the managed Lab gateway
configuration from `~/.config/taxiway/env`. Gateway credentials are not placed
in command arguments, and missing gateway configuration stops the launch.
The launcher also reloads the selected Claude authentication mode. In
`auth_mode=api-key`, every role and handoff receives the gateway key as the
native `ANTHROPIC_AUTH_TOKEN` credential. Subscription mode clears that managed
override and uses the saved native OAuth login.
This does not change Codex configuration or other Claude permission dialogs.

## Settings

The adapter exposes these settings through `--set`:

| Setting | Description | Default |
|---|---|---|
| `version` | Gas Town release version or tag to install | `latest` |
| `beads-version` | [Beads](https://github.com/gastownhall/beads) (Gas Town's Git-backed work-tracking unit) version override; omitted uses the Gas Town compatibility matrix | Adapter default |
| `claude-code-version` | Claude Code CLI release used by Gas Town agents: `latest` or an exact release; distinct from `version` | `latest` |
| `model` | Claude Code model name passed through LiteLLM | `claude-opus-5-5` |
| `tool-search` | Load MCP tool definitions on demand: `true`, `false`, `auto`, or `auto:N` | `true` |
| `claudeai-mcp-servers` | Import Claude.ai connectors: `true` or `false` | `false` |

Settings persist with the lab; `--clear-set` restores the default. MCP settings
apply at each agent launch, including handoffs. Already running agents must
restart or handoff to pick up changes. See [Claude Code settings](claude-code.md#settings)
for connector examples and [Agent Version](claude-code.md#agent-version) for
`claude-code-version`.

Example:

```bash
taxiway up mylab --type gastown --set version=1.2.1 --set model=claude-sonnet-5-5
```

For Gas Town 1.2.1, Taxiway builds the pinned release source with the
[upstream self-sling correction](https://github.com/gastownhall/gastown/pull/4050).
This lets Deacon attach its initial patrol without interrupting its own Claude
session. The first installation downloads a verified Go toolchain and builds
`gt`; its version identifies the backport as `1.2.1-taxiway-self-sling-4050`.

## Shell Behavior

`taxiway shell <lab>` opens an interactive shell in the Gas Town crew directory:

```text
$HOME/gt/<rig>/crew/<lab>
```

From this shell:

| Command | Description |
|---|---|
| `gt mayor attach` | Join the Mayor session |
| `Ctrl-b d` | Detach from the Mayor and return to the shell while preserving the session |
| `gt rig list` | List Gas Town rigs |
| `gt crew status` | Show crew status |

Without `--repo`, the shell opens in `$HOME`.

## Rig And Crew Name Sanitization

Gas Town rejects hyphens, dots, spaces, and path separators in rig and crew
names. `taxiway up` automatically sanitizes these names before injecting them
into the environment.

Rules:

- any byte outside `[A-Za-z0-9_]` is replaced with `_`;
- repo `agentic-clm-demo` becomes rig `agentic_clm_demo`;
- the substitution is logged when it changes the name.

The crew directory path is therefore:

```text
$HOME/gt/agentic_clm_demo/crew/<lab_sanitized>
```

Two repositories whose basenames sanitize to the same name, such as `foo-bar`
and `foo_bar`, cannot coexist as separate rigs in the same lab. `workspace.sh`
detects the collision when the rig name is the same but the `origin` URL
differs, then fails with an explicit error.

Use distinct lab names or rename one repository upstream to avoid the
post-sanitization collision.

## Inspect the Adapter

```bash
taxiway describe gastown
```

## Autonomous permissions

Gas Town requires YOLO/bypass for every automated role. Taxiway preserves
`--dangerously-skip-permissions` during daemon launches and handoffs;
this adapter does not offer a restrictive permission mode.

See [guest capabilities](../contributing/live-tests.md#guest-capabilities).
