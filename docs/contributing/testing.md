# Testing

Taxiway's E2Es prevent regressions in the lifecycle of each orchestrator on
Docker. Live validation lets an agent simply test the feature it is developing
on Docker or Lima, chosen according to the feature. Unit, shell and site tests
cover their respective contracts. Choose the checks that match your change.

To verify a published release across platforms and drivers, see
[Installation qualification](installation-qualification.md).
For pre-publication source, documentation, live and packaging checks followed
by published-artifact checks, use the [release recipe](release-qualification.md).
The evidence helper is tested with `make test-release-tools` (no labs/accounts).

## Choosing tests

| Suite | What it checks | What you need |
|---|---|---|
| Unit | Go behavior, driver commands, configuration, and phase edge cases | Go; no Docker or Lima |
| Shell scripts | Installer and runtime script contracts | Local shell tools; no running lab |
| End-to-end | Nonregression in each orchestrator's lifecycle, including its configured gateway | Go and Docker |
| Live validation | Task-specific manual checks by an agent using reusable tooling | Python, Docker/Lima; account login when inference is needed |
| Site | Documentation routing, navigation, rendering, and landing page content | Node.js and the site dependencies |

Unit and shell tests provide quick feedback during development. End-to-end
tests cover what happens when an orchestrator is provisioned and operated
inside a lab.

Durable E2E assertions enrich the nine existing scenarios in
`internal/cli/orchestrator_e2e_test.go`. Add helpers to that same file; do not
create another scenario file, standalone suite, `TestE2E_` entry point, CI job
or Makefile runner. `e2e_support_test.go` owns setup/cleanup, not a parallel suite.

## Running tests

### Local checks

```bash
make test-unit
make test-scripts
```

`make test` is an alias for `make test-unit`, which runs `go test ./...`.
It does not include shell, site, or end-to-end tests.

For documentation or site changes:

```bash
cd site
npm ci
npm test
npm run build
```

The site renders Markdown from `docs/` directly. Check links and navigation
as well as the production build when adding or moving a page.

### End-to-end

Start Docker, then choose the scope you need:

| Command | Scope |
|---|---|
| `make test-e2e` | All Go tests, including end-to-end tests |
| `make test-e2e-only` | Only end-to-end tests |
| `make test-e2e-up` | `taxiway up` across orchestrators |
| `make test-e2e-prepare-run` | Separate prepare and run across orchestrators |
| `make test-e2e-phase-by-phase` | Individual lifecycle phases across orchestrators |

To focus on one orchestrator, use `make test-e2e-<orchestrator>`, with
`claude-code`, `codex`, or `gastown`. Append `-up`, `-prepare-run`, or
`-phase-by-phase` to narrow it further:

```bash
make test-e2e-codex-up
```

Start with the `up` scenario, then `prepare-run`, then individual phases when
investigating a lifecycle failure. Override the Go test timeout for slow
downloads or first-run setup:

```bash
E2E_TIMEOUT=1800s make test-e2e-gastown-up
```

Docker-backed end-to-end tests skip when Docker is unavailable. To exercise
the skip path explicitly:

```bash
LAB_NO_DOCKER=1 make test-e2e-only
```

A skipped test is not evidence that the lifecycle works.

Before merging a change to provisioned or runtime behavior, run the affected
existing E2E scenarios and verify that they pass on the proposed code. For an
agent change shared by several orchestrators, cover each consuming orchestrator.
Use `up`, `prepare-run`, and `phase-by-phase` when all three paths are affected.
Passing core CI does not replace this check: the GitHub E2E workflow runs on a
schedule or manual dispatch. A skipped or blocked run must be reported and
resolved before claiming E2E validation.

### Task-specific live validation

An agent uses `tests/live/taxiway_live.py` to verify the feature in a real lab,
as a person would manually. The [live guide](live-tests.md) explains auth reuse,
bounded guest commands, temporary labs and cleanup. The
[model gateway guide](model-gateway-tests.md) describes evidence for real model
selection, delegation, tools and patrols. Temporary feature scripts and sanitized
reports stay outside Git; do not grow a committed live regression suite.

Real model requests consume account usage and are explicitly opt-in, outside
unattended CI. Persistent labs keep generic names `test-claude`, `test-codex`,
`test-gastown` (or their existing names) and remain available for manual checks.
Remove only owned temporary labs automatically.

### Disposition of former live scenarios

| Former script | Durable E2E coverage | Task-specific real-account evidence |
|---|---|---|
| `test_recording_lifecycle.py` | Existing Go recording scenarios cover Docker capture/prefix/containment/recovery. Exceptional `temporary_lab` cleanup belongs to helper safety tests. Native Lima observations are selected live recipes, not covered by Docker | Readiness/mounts/recording/containment/recovery and ownership observations in the live guide; scripts remain temporary |
| `test_lima_startup.py` | The current nine scenarios use Docker; Lima fresh readiness, boot-marker rejection/retry and restart are live recipes | Workflow skill routes to bounded live recipes; do not claim automated Lima coverage |
| `test_claude_models.py` | Existing gateway/orchestrator E2Es cover protocol routing, model configuration, permission defaults and restart | Actual alias/full-ID access, inherited/explicit child models, tools/build/HTTPS, OAuth reuse after restart |
| `test_codex_models.py` | Existing gateway/orchestrator E2Es cover Responses routing, model configuration, permission defaults and restart | Actual principal/child model access, session relationships, gateway tokens and delegated tool effects |
| `test_gastown.py` | Existing Gas Town E2Es cover workspace/roles, model/alias/gateway environment, self-sling and handoff process/configuration preservation | Actual patrol checks/report/heartbeat progress, fresh startup, progress after Deacon handoff, launcher inference and delegation |
| `gateway_protocol_e2e_test.go` and its Python runner | Protocol assertions are part of the existing gateway-routed steps, using each lab's actual sidecar and controlled upstream, including its database/telemetry | No real-account claim |

The real-account column is documented manual validation, not automated regression
coverage. Removing the fixed scripts does not make those claims pass under a
controlled provider; choose and execute relevant observations explicitly.

## Test coverage

### Unit and shell tests

Unit tests cover driver command construction using fake executables, runtime
path resolution, configuration, gateway handling, and lifecycle edge cases.
They must not operate on real Docker or Lima resources.

Shell tests live under `tests/scripts/`. They cover the standalone installer,
workspace trust, script output, reset behavior, event emission, Git cloning,
and orchestrator workspace/start scripts. `make test-scripts` discovers
`test_*.sh` files recursively.

### End-to-end

Session checks are Go helper functions in `internal/cli/orchestrator_e2e_test.go`.
They execute commands through the driver and assert their results directly.
Do not add a separate unit-test layer for E2E assertions.
Tests of the shipped launcher and profile remain under `tests/scripts/`.

Keep Lab lifecycle checks independent of orchestrator-specific expectations.
Agent checks follow the agents declared in the orchestrator manifest: the same
Claude Code trust assertion applies to both the Claude Code and Gas Town
orchestrators. Orchestrator expectations select the workspace paths and when
they should be trusted. Standalone agents use `/lab/work/agreement-hub`; Gas
Town uses its rig's crew workspace, trusted when the agent is launched.
`/lab/work` is checked after installation for every declared agent supported
by these scenarios. Gas Town session diagnostics remain separate from the
shared `shell --check` assertion.

The end-to-end suite exercises `claude-code`, `codex`, and `gastown` through
the Docker driver, using their real orchestrator and agent assets.

The scenarios cover installation and verification inside the lab, gateway
access, workspace preparation, start, stop, restart, diagnosis, reset, and
removal. They mirror the public `manufacture-dev/agreement-hub` fixture into a
lab-local bare Git repository, then clone the working tree from that isolated
remote. After start and restart, `taxiway shell <lab> --check` verifies that
the session target is ready without opening an interactive shell.

For Gas Town, the same scenarios also compare present persistent agent sessions
with `gt status --json` and inspect the zombie check from `gt doctor` (without
`--fix`). They inspect the startup doctor log as well, so deleting falsely
classified zombies during startup cannot turn the check green. These assertions
do not require Boot or idle agents to be present. Role status alone does not
prove inference; separate interactive request checks establish completion with
the controlled upstream, while real-model completion requires live validation.

The Gas Town phase-by-phase scenario also renews the refinery twice and the
Mayor once with `gt handoff`. It checks that Claude replaces the previous
process and retains its configured model and gateway environment. This covers
session renewal separately from stopping and starting the Lab.

Claude Code and Gas Town share an assertion that reads the actual Claude
process environment after initial start and after Lab restart. It verifies that
tool search is enabled and Claude.ai connector import is disabled by default.
The same assertion runs after each Gas Town handoff. Overrides and clearing
settings are covered by the runtime script tests.

Model expectations come from the catalog and orchestrator manifests of the tested commit.
Provider requests are simulated locally with fake credentials; no online model discovery is needed.
Existing scenarios verify model defaults, explicit selections, provider exposure, routing, and restart/handoff propagation.
Real principal/subagent delegation requires task-specific [live observations](model-gateway-tests.md).

Tests use `--skip-auth-check`. They do not run interactive authentication,
use real API keys, or exercise browser/device login. Authenticated execution
depends on external accounts and interactive state and is outside this suite.

## CI structure

| Workflow | Trigger | Coverage |
|---|---|---|
| `ci.yml` | Push, pull request, or manual | Build, Go unit tests, shell tests, and lints |
| `e2e.yml` | Daily schedule or manual | Source-tree orchestrator lifecycles through Docker |

End-to-end tests run separately from the fast core checks. Their matrix uses
`fail-fast: false`, so one failure does not cancel the other orchestrators.

The end-to-end workflow has one Ubuntu job per orchestrator. Each job runs
`up`, `prepare-run`, and `phase-by-phase` in that order.

Run the site checks locally for documentation and frontend changes; they are
not part of `ci.yml`.

For end-to-end failures, inspect the failing orchestrator's `up`, `prepare-run`,
or `phase-by-phase` step, then rerun that scope locally.

## Adding or updating tests

When a feature changes what a lab receives or does, enrich the relevant existing
E2E scenarios with assertions of that added behavior. A process being alive, a
successful CLI exit, or a gateway health check does not prove model selection,
configuration propagation, or the effect of an agent action. Assert observable
configuration or results inside the lab; include restart/handoff paths when the
behavior must survive session renewal. Keep expected values independent of the
production code that selects or renders them. Perform task-specific live validation
when the behavior requires real provider requests, without replacing the
credential-free E2E assertions or committing a feature-specific live suite.

| Change | Where to add coverage |
|---|---|
| Go behavior or driver command | Nearest `_test.go` file, with no build tag; verify with `make test-unit` |
| Shell behavior | A `test_*.sh` file under `tests/scripts/`; verify with `make test-scripts` |
| Orchestrator lifecycle or gateway protocol | Enrich the nine existing scenarios and helpers in `internal/cli/orchestrator_e2e_test.go`; run the affected existing targets |
| Real-account agent or feature behavior | Temporary manual check outside Git using `taxiway_live.py`; run explicitly in a dev/e2e context |
| Documentation page or site navigation | Existing tests under `site/src/`; run site tests and build |

The existing Go E2E files start with the following directive and a blank line:

```go
//go:build e2e

```

Keep the nine existing `TestE2E_` functions; extend their current steps instead
of adding entry points. Local Docker availability checks may use `requireDockerOrSkip(t)`.

When adding documentation pages, test routes, links, and navigation rather
than asserting a fixed total page count.
