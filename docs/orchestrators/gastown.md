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

## Settings

The adapter exposes these settings through `--set`:

| Setting | Description | Default |
|---|---|---|
| `version` | Gas Town version or tag to install from release archive | `latest` |
| `beads-version` | [Beads](https://github.com/gastownhall/beads) (Gas Town's Git-backed work-tracking unit) version override; omitted uses the Gas Town compatibility matrix | Adapter default |
| `model` | Claude Code model name passed through LiteLLM | `claude-opus-4-8` |

Example:

```bash
taxiway up mylab --type gastown --set version=1.1.0 --set model=claude-sonnet-4-6
```

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
