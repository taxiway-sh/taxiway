# Configuration

Taxiway separates installed runtime assets from mutable user state.

## Runtime Assets

Release installs place runtime assets under:

```text
~/.taxiway/runtime
```

The runtime directory contains files needed to create labs:

- `infra/`
- `agents/`
- `orchestrators/`

Override it with `TAXIWAY_RUNTIME_DIR` when developing from a source checkout.

## Lab State

Lab state defaults to:

```text
~/.taxiway/lab-state
```

Override it with `--state-dir <path>` or `TAXIWAY_LAB_STATE_DIR`.

When running Taxiway from a source checkout, an `.envrc` redirects all state
paths into the checkout — see [Development](../contributing/development.md).

## Drivers

Taxiway supports:

| Driver | Requirement |
|---|---|
| `lima` | `limactl` available on `PATH` |
| `docker` | `docker` on `PATH` and daemon |

Force a driver for one command:

```bash
taxiway up mylab --driver docker
```

Set a default driver for the shell:

```bash
export LAB_DRIVER=docker
taxiway up mylab
```

Driver resolution order is:

1. `--driver <name>`
2. `LAB_DRIVER`
3. auto-detection: Lima when `limactl` is available, then Docker

See [Drivers](../README.md#drivers) for Lima and Docker details.

## Gateway

Taxiway starts host-local gateway pieces automatically from lab commands such as
`taxiway up`, `taxiway run`, and `taxiway gateway <lab>`. The gateway path uses
the shared proxy plus one LiteLLM sidecar per lab.

Lab-specific gateway state lives under each lab:

```text
~/.taxiway/lab-state/<lab>/gateway
```

Converted provider auth used by gateways lives under:

```text
~/.taxiway/auth
```

See [Gateway](../how-to/gateway.md).

## Observability

Taxiway can manage an optional Langfuse observability stack:

```bash
taxiway observe up
taxiway observe open
```

Initialize the full host-local runtime with:

```bash
taxiway init
```

`taxiway init` starts the shared proxy and Langfuse. It does not create a lab.

Langfuse is exposed through the Taxiway proxy at
`http://langfuse.localhost:<proxy-port>`. The observability Compose stack does
not publish its own host ports.

Mutable observability state lives under:

```text
~/.taxiway/observability
```

Proxy state lives under:

```text
~/.taxiway/proxy
```

See [Observability](../how-to/observability.md).

## Shell Completion

Generate completion for the installed CLI:

```bash
taxiway completion zsh > ~/.zsh/completions/_taxiway
taxiway completion bash > ~/.bash_completion.d/taxiway
taxiway completion fish > ~/.config/fish/completions/taxiway.fish
```
