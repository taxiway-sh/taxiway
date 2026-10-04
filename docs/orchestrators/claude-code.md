# Claude Code

The `claude-code` adapter installs and runs Anthropic's Claude Code CLI directly
inside a Taxiway lab, with LiteLLM routing enabled.

## Quick Start

```bash
taxiway up mylab --type claude-code --repo https://github.com/org/repo
taxiway shell mylab
```

`taxiway shell mylab` attaches to the Claude Code tmux session. Send prompts
directly in that session.

The recommended lab path is to route Claude Code through LiteLLM. For Claude
Code Max subscriptions, Claude Code can forward its subscription OAuth token
through LiteLLM. The lab only needs the LiteLLM gateway key as a secret; endpoint
and model routing are orchestrator configuration. Run `taxiway observe up`
separately when you also want Langfuse traces.

For direct OAuth login, run:

```bash
taxiway auth mylab claude-code
```

If host OAuth credentials exist at `~/.claude/.credentials.json`, Taxiway can
copy them into the lab during `taxiway auth`. Claude Code Max still needs
a Claude Code OAuth token in the lab so LiteLLM can forward the
`Authorization` header to Anthropic.

## Settings

The adapter exposes these settings through `--set`:

| Setting | Description | Default |
|---|---|---|
| `claude-code-version` | Claude Code CLI release: `latest` or an exact release | `latest` |
| `model` | Claude Code model name passed through LiteLLM | `claude-opus-5-5` |
| `tool-search` | Load MCP tool definitions on demand: `true`, `false`, `auto`, or `auto:N` | `true` |
| `claudeai-mcp-servers` | Import Claude.ai connectors: `true` or `false` | `false` |

Tool search reduces context pressure when many MCP tools are available.
Claude.ai connector import is disabled by default. Enable it explicitly when
the lab needs access to connected services. Other configured MCP servers remain
available. Clearing the setting disables Claude.ai connector import again.

```bash
taxiway start mylab --set claudeai-mcp-servers=true
taxiway start mylab --clear-set claudeai-mcp-servers
```

Settings persist with the lab; `--clear-set` restores the default.
`taxiway start` restarts the Claude Code session with the updated settings.

### Agent Version

`claude-code-version` selects the Claude Code CLI release installed in the lab. It is
independent of `model`, which selects the LLM.

| Value | Behavior |
|---|---|
| omitted or `latest` | Install the latest release in a fresh lab; keep whatever release is already installed afterwards. Not an immutable pin. |
| exact release, e.g. `2.1.288` | Install exactly this release of `@anthropic-ai/claude-code`, upgrading or downgrading an existing installation. Claude Code launches, including authentication, run with `DISABLE_AUTOUPDATER=1` so the release cannot change itself. |

```bash
taxiway up mylab --type claude-code --set claude-code-version=2.1.288
taxiway install mylab --set claude-code-version=<other-release>
taxiway install mylab --clear-set claude-code-version
```

The pin persists with the lab, so stop/start, phase resumes, and reinstalls use
the same release. Changing it reinstalls the agent without touching the
workspace or credentials. Clearing it returns to `latest` and keeps the
installed release.

Only `latest` and exact releases are accepted; ranges and other npm tags are
rejected. An unknown release fails the install phase without falling back to
another release. The verify phase prints `claude --version`; for an unpinned lab,
use that release to pin the same one in another lab.

The gateway model catalog requires Claude Code 2.1.284 or newer (see
[Gateway](../how-to/gateway.md)); start fails with an older pin.

## Agent CLI

The adapter uses the `claude-code` agent, which installs the npm package
`@anthropic-ai/claude-code` and verifies `claude --version`, `claude --help`, and
auth configuration readability without making an API call.

## Inspect the Adapter

```bash
taxiway describe claude-code
```

`taxiway list <lab>` shows the requested harness version and the last observed
installed release and executable path. Installation and `taxiway verify <lab>`
refresh that observation in the lab's `agent-versions.json` state; the requested
version remains in its saved settings. Verification fails if the executable on
the launch PATH differs from an exact pin.

## Autonomous permissions

Claude Code runs with `--dangerously-skip-permissions` inside the guest.
The launcher suppresses the separate initial bypass warning; authentication
and onboarding remain independent prerequisites.

See [guest capabilities](../contributing/live-tests.md#guest-capabilities).
