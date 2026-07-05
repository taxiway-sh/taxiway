# Observability

Taxiway's observability stack is the optional Langfuse runtime used to store and
inspect model traces. It is separate from lab gateways:

- **Observability stack:** Langfuse web, Langfuse worker, Postgres,
  ClickHouse, Redis, and MinIO.
- **Gateway path:** the shared Caddy proxy plus one LiteLLM sidecar per lab.

The observability stack does not proxy model traffic. Lab gateways can export
LiteLLM traces to Langfuse when observability is running, but they can also run
without Langfuse.

For the gateway side, see [Gateway](gateway.md).

## Prerequisites

- `docker` on `PATH` and a reachable Docker daemon

Langfuse runs on Docker regardless of the selected lab driver. Lima-backed labs
still use Docker for host-local services such as the shared proxy, lab gateways,
and observability.

## Quick Start

Start the full local runtime:

```bash
taxiway init
taxiway observe open
```

`taxiway init` starts the shared proxy and the Langfuse observability stack. It
does not create a lab and does not start a lab LiteLLM gateway.

If you only want Langfuse and the proxy:

```bash
taxiway observe up
taxiway observe open
```

To generate traces, start or refresh a lab gateway separately:

```bash
taxiway gateway <lab>
```

Then run model traffic through the lab gateway. Successful LiteLLM calls appear
in Langfuse when the lab has generated Langfuse keys.

## Access

Langfuse is exposed through the shared Taxiway proxy:

```text
http://langfuse.localhost:<proxy-port>
```

Use the live command output instead of guessing the port:

```bash
taxiway access
taxiway status
```

When Taxiway is installed, the default proxy endpoint is usually:

```text
http://langfuse.localhost:4000
```

## Commands

| Command | Scope | Description |
|---|---|---|
| `taxiway init` | runtime | Start the shared proxy and Langfuse observability |
| `taxiway observe up` | observability | Start Langfuse and generate `.env` on first run |
| `taxiway observe down` | observability | Stop Langfuse containers and preserve runtime state and volumes |
| `taxiway observe rm` | observability | Remove Langfuse containers and clear observability runtime state |
| `taxiway observe rm --volumes` | observability | Also remove observability data volumes |
| `taxiway observe reset` | observability | Remove volumes, clear runtime state, and restart Langfuse |
| `taxiway observe open` | observability | Open Langfuse through the shared proxy |
| `taxiway status` | runtime | Show labs, proxy, gateways, and observability status |
| `taxiway access` | runtime | Print Langfuse and gateway endpoints |

Gateway commands such as `taxiway gateway <lab>`, `taxiway up <lab>`, and
`taxiway run <lab>` are documented in [Gateway](gateway.md).

## Runtime State

Installed runtimes store mutable observability state under:

```text
~/.taxiway/observability
```

The generated observability environment file is:

```text
~/.taxiway/observability/.env
```

The Langfuse Compose file is a runtime asset under:

```text
~/.taxiway/runtime/infra/observability/langfuse.compose.yml
```

The shared proxy state is separate:

```text
~/.taxiway/proxy
```

When running from a source checkout, these paths are redirected into the
checkout and the Compose project and proxy container are namespaced per checkout
— see [Development](../contributing/development.md).

## Relationship To Gateways

Langfuse receives traces from LiteLLM. It does not manage provider credentials,
model routing, or lab gateway keys.

The lab `gateway` phase is responsible for:

- generating the lab-specific LiteLLM API key;
- creating the lab LiteLLM sidecar and gateway Postgres database;
- registering the `<lab>.litellm.localhost` route in the shared proxy;
- generating local Langfuse ingestion keys for that lab when observability is
  initialized.

`taxiway observe up` only starts Langfuse. It does not create lab gateway keys
and does not start LiteLLM sidecars.

Taxiway writes its own lab lifecycle events (`LAB_AGENT_EVENT` records) to the
lab's local `events.jsonl` file. They are not sent to Langfuse. Langfuse is
reserved for agent/model traffic collected through LiteLLM.

## Reset

```bash
taxiway observe down          # stop only; keep containers, volumes, and runtime state
taxiway observe rm            # remove containers and clear runtime state
taxiway observe rm --volumes  # also remove observability data volumes
taxiway observe reset         # rm --volumes, then restart
```

`taxiway observe down` is non-destructive. It stops Langfuse and preserves the
runtime state so `taxiway observe up` can restart it through the proxy.

`taxiway observe rm` removes stack containers and clears the observability
runtime marker in dev/e2e contexts. With `--volumes`, Docker also removes
observability data volumes.

`taxiway observe reset` wipes the Postgres volume: **all traces are permanently
deleted**. It preserves the observability `.env` by default and only regenerates
missing values. Use `--rotate-secrets` if you explicitly want to regenerate
observability credentials.

## Troubleshooting

| Symptom | Check |
|---|---|
| Langfuse UI inaccessible | Run `taxiway status`; verify Docker, the shared proxy, and the Langfuse stack |
| Langfuse starts but health is slow | Check `docker logs` for `langfuse-web`; first startup runs database migrations |
| Traces are missing | Verify the lab gateway is running, then check [Gateway](gateway.md) |
| `docker daemon is not running` | Start Docker Desktop or run `dockerd &` |
