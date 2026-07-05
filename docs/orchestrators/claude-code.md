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

## Agent CLI

The adapter uses the `claude-code` agent, which installs the npm package
`@anthropic-ai/claude-code` and verifies `claude --version`, `claude --help`, and
auth configuration readability without making an API call.

## Inspect the Adapter

```bash
taxiway describe claude-code
```
