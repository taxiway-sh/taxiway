# Lima

The Lima driver creates one Lima-backed lab per Taxiway lab. It is the preferred
local driver when `limactl` is available.

## Requirements

- `limactl` on `PATH`.
- A host that can run Lima instances.

Taxiway auto-selects Lima before Docker when both are available.

The shared gateway is not part of the Lima VM. It runs on the host through
Docker and provides Caddy plus per-lab LiteLLM sidecars. Langfuse observability
also runs on the host through Docker when started with `taxiway observe up`.

## Template

The driver renders a Lima VM template into the lab state directory before
running:

```bash
limactl start --name=<id> <rendered-yaml>
```

The rendered YAML is kept with the lab state for debugging.

## Mounts

```mermaid
flowchart LR
  runtime[Runtime assets] --> infra["/lab/infra"]
  runtime --> agents["/lab/agents"]
  runtime --> orch["/lab/orchestrators/<type>"]
  state --> git["/lab/git-source read-only"]
  state --> recordings["/lab/recordings"]
  internal[Internal lab state] --> work["/lab/work"]
  internal --> fork["/lab/git writable"]
```

The template mounts only the runtime assets needed inside the lab:

- `/lab/infra`
- `/lab/agents`
- `/lab/orchestrators/<type>`
- `/lab/git-source` (read-only)
- `/lab/recordings`

`/lab/work` and `/lab/git` are created inside the lab and are not host-mounted.
Working trees and writable Git forks live there. `/lab/git-source` exposes
host source mirrors read-only; `/lab/recordings` remains the host-visible
writable recordings mount.

The template intentionally does not define a top-level Lima `users:` block, so
Lima keeps its default host-user mapping.

## Commands

The driver shells out to `limactl` for lifecycle, copy, shell, and exec
operations.
