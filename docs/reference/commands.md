# CLI Usage

Run `taxiway help` or `taxiway help <command>` for the live CLI reference.

## Lifecycle

| Command | Description |
|---|---|
| `taxiway up <lab> [--type <orch>]` | Run prepare then runtime phases |
| `taxiway prepare <lab> [--type <orch>]` | Run `create`, `bootstrap`, `install`, `verify` |
| `taxiway run <lab>` | Run `gateway`, `workspace`, `auth`, `start` |

## Lifecycle Phases

| Command | Description |
|---|---|
| `taxiway create <lab> [--type <orch>]` | Create the lab |
| `taxiway bootstrap <lab>` | Install system dependencies |
| `taxiway install <lab>` | Install the orchestrator and agents |
| `taxiway verify <lab>` | Verify the orchestrator and agents |
| `taxiway gateway <lab>` | Reconcile generated lab gateway access |
| `taxiway workspace <lab>` | Prepare the lab workspace |
| `taxiway auth <lab> [agent...]` | Run lab agent authentication interactively |
| `taxiway start <lab>` | Start lab runtime sessions |

## Labs

| Command | Description |
|---|---|
| `taxiway list [<lab>]` / `taxiway ls [<lab>]` | List labs, or show one lab |
| `taxiway record` | Manage lab recordings |
| `taxiway record start <lab> [--name <name>]` | Start recording the lab shell |
| `taxiway record stop <lab> [--name <name>]` | Stop a running recording |
| `taxiway record list [<lab>]` | List recordings for one lab, or all labs |
| `taxiway record player <lab>` | Serve the browser player for lab recordings |
| `taxiway record analyze <lab>` | Analyze stopped recordings with a local agent runner |
| `taxiway record rm <lab> <name> [--force]` | Remove a recording and its cast file |
| `taxiway shell <lab>` | Attach to the lab shell or orchestrator session |
| `taxiway shell <lab> --check` | Verify that the lab session target is ready without attaching |
| `taxiway doctor <lab>` | Diagnose the lab environment |
| `taxiway down <lab>` | Stop a lab and preserve its state |
| `taxiway rm <lab>` | Delete a lab |
| `taxiway reset <lab> [--yes]` | Reset a lab and clear phase markers |

## Runtime

| Command | Description |
|---|---|
| `taxiway init` | Initialize the Taxiway runtime, including proxy and observability |
| `taxiway status` | Show global Taxiway runtime status: labs, proxy, lab gateways, and optional Langfuse |
| `taxiway access` | Show service URLs and credentials for Langfuse and lab gateways |
| `taxiway repair` | Restore generated Taxiway runtime state and restart an initialized proxy if needed |
| `taxiway destroy` | Destroy labs, gateways, observability, proxy, volumes, and generated runtime state |
| `taxiway credentials codex` | Prepare host Codex auth for LiteLLM gateways |
| `taxiway observe up` | Start optional local Langfuse trace storage |
| `taxiway observe down` | Stop the observability stack and preserve containers, volumes, and runtime state |
| `taxiway observe rm` | Remove observability stack containers and clear runtime state |
| `taxiway observe rm --volumes` | Remove observability stack containers, data volumes, and runtime state |
| `taxiway observe reset` | Remove volumes, clear runtime state, and restart |
| `taxiway observe open` | Open Langfuse UI through the Taxiway proxy |

## Utility

| Command | Description |
|---|---|
| `taxiway describe <orchestrator>` | Describe an orchestrator adapter |
| `taxiway version` | Show CLI version and runtime paths |
| `taxiway completion <shell>` | Generate shell completion |
| `taxiway help [command]` | Show help |

## Common Flags

| Flag | Description |
|---|---|
| `--driver <name>` | Force a driver: `lima` or `docker` |
| `--dry-run` | Print phases without executing |
| `--state-dir <path>` | Override state directory |
| `-v`, `--verbose` | Enable verbose logging |

`LAB_DRIVER` can set the default driver for the shell when `--driver` is not
provided. The flag takes precedence over the environment variable.
