# Understanding Taxiway

Taxiway creates isolated labs for running and comparing AI agent orchestrators
on Lima or Docker, each with a LiteLLM gateway, Caddy proxy, and optional
Langfuse observability. This documentation covers how to install, use,
understand, and contribute to the tool.

## Reference

| Page | Purpose |
|---|---|
| [Concepts](reference/concepts.md) | Core product model |
| [CLI Usage](reference/commands.md) | CLI command reference |
| [Configuration](reference/configuration.md) | Runtime paths, drivers, and observability |
| [Architecture](reference/architecture.md) | System design |

## Drivers

Drivers create and operate labs.

| Driver | Best fit | Requirement | Details |
|---|---|---|---|
| `lima` | Local labs on macOS and Linux | `limactl` on `PATH` | [Lima](drivers/lima.md) |
| `docker` | Container-backed labs, Linux development, Docker-backed end-to-end coverage | `docker` on `PATH` and daemon | [Docker](drivers/docker.md) |

Taxiway auto-selects Lima when `limactl` is available, then Docker when Docker is
available.

Use `--driver <name>` to force a driver for one command, or set `LAB_DRIVER` to
choose a default driver for the current shell.

## Orchestrators

Orchestrator adapters define what gets installed, verified, authenticated, and
started inside a lab.

| Type | Description | Supported Agents | Details |
|---|---|---|---|
| `claude-code` | Anthropic Claude Code CLI | Claude Code | [Claude Code](orchestrators/claude-code.md) |
| `codex` | OpenAI Codex CLI | Codex | [Codex](orchestrators/codex.md) |
| `gastown` | Gas Town HQ and workspace shell | Claude Code | [Gas Town](orchestrators/gastown.md) |

Inspect what an adapter provides for a given type:

```bash
taxiway describe <type>
```

Typical flow:

1. Choose an adapter type.
2. Run `taxiway up <lab> --type <type>`, optionally with `--repo <url>`.
3. Complete adapter authentication when required.
4. Attach to the lab with `taxiway shell <lab>`.
5. Run the workflow in the attached shell or orchestrator session.

## How-to

- [Gateway](how-to/gateway.md)
- [Observability](how-to/observability.md)
- [Recordings](how-to/recordings.md)

## Contributing

- [Development](contributing/development.md)
- [Testing](contributing/testing.md)
- [Release](contributing/release.md)
