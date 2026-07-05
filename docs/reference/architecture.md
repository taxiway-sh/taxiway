# Architecture

Taxiway is a CLI-first lab runner. Its core job is to turn a requested lab into
a reproducible environment by selecting a driver, persisting lab metadata,
executing lifecycle phases, and delegating orchestrator-specific behavior to
adapter scripts.

## System Overview

```mermaid
flowchart TD
  user[User] --> cli[taxiway CLI]
  cli --> config[Config resolver]
  cli --> phases[Phase runner]
  cli --> gateway[Gateway runtime]
  cli --> observe[Observability runtime]
  phases --> driver[Driver interface]
  driver --> lima[Lima driver]
  driver --> docker[Docker driver]
  phases --> adapters["orchestrators/&lt;type&gt; scripts"]
  adapters --> agents["agents/&lt;agent&gt; scripts"]
  config --> state[(~/.taxiway/lab-state)]
  gateway --> proxy[(Caddy proxy + lab LiteLLM)]
  observe --> langfuse[(Langfuse, optional)]
```

The Go CLI owns command parsing, phase ordering, state paths, driver selection,
event parsing, and user-facing output. Drivers own the mechanics of creating
and operating labs. Orchestrator and agent scripts own tool-specific setup
inside the lab.

## Main Components

| Component | Responsibility |
|---|---|
| `cmd/taxiway` | CLI entry point |
| `internal/cli` | Cobra commands, global flags, lifecycle orchestration |
| `internal/config` | Runtime paths, state paths, lab metadata, orchestrator validation |
| `internal/phases` | Canonical phase names, ordering, and phase marker files |
| `internal/driver` | Driver interface shared by Lima, Docker, and dry-run execution |
| `internal/event` | `LAB_AGENT_EVENT` parsing and formatting |
| `internal/cli/proxy.go` | Shared Caddy proxy runtime, route registry, and generated Caddyfile |
| `internal/cli/lab_gateway.go` | Per-lab LiteLLM sidecar, Postgres, route, and gateway environment generation |
| `orchestrators/<type>` | Adapter scripts for install, verify, workspace, auth, start, doctor |
| `agents/<agent>` | Agent CLI install, verify, auth, and doctor scripts |
| `infra/` | Runtime assets copied or mounted into labs |

## Runtime Assets and User State

Taxiway separates immutable runtime assets from mutable user state.

```mermaid
flowchart LR
  release[Installed release] --> runtime["~/.taxiway/runtime"]
  source[Source checkout] --> envrc[.envrc overrides]
  envrc --> runtimeDev[TAXIWAY_RUNTIME_DIR=$PWD]
  envrc --> authDev[TAXIWAY_AUTH_DIR=$PWD/.auth]
  envrc --> proxyDev[TAXIWAY_PROXY_DIR=$PWD/.proxy]
  envrc --> observeDev[TAXIWAY_OBSERVABILITY_DIR=$PWD/.observability]
  envrc --> labDev[TAXIWAY_LAB_STATE_DIR=$PWD/.lab-state]
  userState[Mutable state] --> labState[~/.taxiway/lab-state]
  userState --> observability[~/.taxiway/observability]
  userState --> auth[~/.taxiway/auth]
  userState --> proxy[~/.taxiway/proxy]
```

Release installs use `~/.taxiway/runtime` for runtime assets.
Source-checkout development sets checkout-local runtime state through `.envrc`.

## Lab Metadata

Each lab has a persisted `ref.json` file under the lab state directory. It is
the source of truth for resuming commands without repeating command-line flags.

```json
{
  "version": 5,
  "lab": "mylab",
  "orch": "codex",
  "driver": "lima",
  "workspace": {
    "repo": "https://github.com/org/repo",
    "ref": "main",
    "path": "subdir"
  },
  "orchestrator_profile": {
    "name": "default"
  },
  "settings": {
    "version": "latest"
  }
}
```

The lab name determines the environment name, usually `taxiway-<lab>`. The
orchestrator type is stored separately so commands such as
`taxiway run <lab>` can resume the correct adapter without another `--type`
flag.

## Phase Execution

`taxiway up` is a phase runner. Each phase has a marker file under the lab state
directory. Completed markers let Taxiway resume interrupted work without
rerunning finished phases.

```mermaid
flowchart LR
  create[create] --> bootstrap[bootstrap]
  bootstrap --> install[install]
  install --> verify[verify]
  verify --> gateway[gateway]
  gateway --> workspace[workspace]
  workspace --> auth[auth]
  auth --> start[start]
```

| Phase | Owner | Purpose |
|---|---|---|
| `create` | Driver | Create the lab environment and write `ref.json` |
| `bootstrap` | Infra command | Install system dependencies |
| `install` | Orchestrator and agents | Install adapter-specific tools |
| `verify` | Orchestrator and agents | Verify installed tools without doing real work |
| `gateway` | CLI + driver | Reconcile generated LiteLLM/proxy access and write lab gateway env |
| `workspace` | Orchestrator adapter | Prepare the workspace repository |
| `auth` | Agent scripts | Run interactive authentication when needed |
| `start` | Orchestrator adapter | Start runtime sessions |

## Driver Boundary

All lab operations go through `internal/driver.Driver`. CLI commands do not call
`limactl`, `docker`, or shell commands directly for lifecycle operations.

```mermaid
flowchart TD
  cli[CLI command] --> iface[driver.Driver]
  iface --> lifecycle[Lab lifecycle]
  iface --> metadata[LabRef read/write]
  iface --> transfer[File copy]
  iface --> exec[Command execution]
  iface --> shell[Interactive shell]
  lifecycle --> lima[Lima]
  lifecycle --> docker[Docker]
```

This boundary keeps lifecycle commands independent from the underlying driver
implementation. Lima and Docker can differ internally while exposing the same
lifecycle, copy, exec, and shell methods to the CLI.

Driver-specific runtime behavior is documented in [Drivers](../README.md#drivers).

## Adapter Boundary

Orchestrator adapters live under `orchestrators/<type>/`. Agent installers live
under `agents/<agent>/`. The adapter manifest declares which agents are
currently supported by that adapter.

```mermaid
flowchart LR
  cli[Phase runner] --> manifest["orchestrators/&lt;type&gt;/manifest.yaml"]
  manifest --> adapterScripts[orchestrator scripts]
  manifest --> agentList[supported agents]
  agentList --> agentScripts["agents/&lt;agent&gt; scripts"]
  adapterScripts --> lab[Lab environment]
  agentScripts --> lab
```

Taxiway owns the lab lifecycle and calls the adapter at known phase boundaries.
The adapter owns the details of installing, verifying, configuring, and starting
the specific orchestrator.

## Events and Observability

Adapter and agent scripts can emit `LAB_AGENT_EVENT {json}` lines. The driver
execution path parses those lines and writes structured events to a local JSONL
file for lab debugging.

```mermaid
flowchart LR
  scripts[Adapter and agent scripts] --> eventLine[LAB_AGENT_EVENT JSON lines]
  eventLine --> parser[internal/event parser]
  parser --> jsonl[Local JSONL events]
```

Langfuse traces come from LiteLLM model traffic, not from Taxiway's internal
phase events.

## Extension Points

| Extension | Files to add or change |
|---|---|
| New orchestrator adapter | `orchestrators/<type>/manifest.yaml` and phase scripts |
| New supported agent CLI | `agents/<agent>/install.sh`, `verify.sh`, `auth.sh`, `doctor.sh` |
| New driver | Implementation of `internal/driver.Driver` and driver selection wiring |
| New phase behavior | `internal/phases`, corresponding CLI command, and driver/adapter calls |
| New local event sink | `internal/event` and driver execution wiring |

The preferred extension path is to add narrow adapters or drivers behind
existing interfaces rather than expanding the CLI command surface first.
