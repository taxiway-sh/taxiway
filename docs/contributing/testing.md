# Testing

Taxiway's E2Es prevent regressions in the lifecycle of each orchestrator on
Docker. Live validation lets an agent simply test the feature it is developing
on Docker or Lima, chosen according to the feature. Unit, shell and site tests
cover their respective contracts. The
[`taxiway-testing` skill](https://github.com/taxiway-sh/taxiway/blob/main/.agents/skills/taxiway-testing/SKILL.md) says which checks a change needs; this page
owns the commands.

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

The [E2E scenario rule](https://github.com/taxiway-sh/taxiway/blob/main/.agents/skills/taxiway-testing/SKILL.md#e2e-scenarios) says where durable
E2E assertions go and which scenarios to run. This page owns how they are
written and run.

### Writing E2E scenarios

Keep E2E scenarios chronological: show each attempted command, native interaction
or injected fixture as a first-level action, with its named guarantees directly
underneath. Shared helpers must not add agent/model wrappers or nested assertion
levels. Attach agent, workspace, model, session and gateway observations to the
action that establishes them; preserve onboarding and readiness ordering.
Assertions observe: make runtime mutations explicit, restore injected fixtures
on failure, and keep protocol/model diagnostics precise without a hidden suite.

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

The site renders Markdown from `docs/` directly. The
[`taxiway-docs` skill](https://github.com/taxiway-sh/taxiway/blob/main/.agents/skills/taxiway-docs/SKILL.md)
owns how pages are written to render on GitHub and on the site: links,
navigation, titles and diagrams.

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
`claude-code`, `codex`, `gastown`, or `gascity`. Append `-up`, `-prepare-run`, or
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

Single-orchestrator targets use `E2E_TIMEOUT` (default `1200s`). Gas City
targets use `E2E_GASCITY_TIMEOUT` (default `3600s`) because each scenario
restarts a full city several times; its phase-by-phase scenario takes about 30
minutes. Aggregate targets (`test-e2e`, `test-e2e-only`, `test-e2e-up`,
`test-e2e-prepare-run`, `test-e2e-phase-by-phase`) run several orchestrators in
sequence and use `E2E_SUITE_TIMEOUT` (default `4h`); all twelve scenarios take
about 2.5 hours. The scheduled E2E workflow runs one orchestrator per job with
a 90-minute limit; the three Gas City scenarios take about 55 minutes.

Docker-backed end-to-end tests skip when Docker is unavailable. To exercise
the skip path explicitly:

```bash
LAB_NO_DOCKER=1 make test-e2e-only
```

A skipped test is not evidence that the lifecycle works.

The GitHub E2E workflow runs on a schedule or manual dispatch, not on pull
requests; the [E2E scenario rule](https://github.com/taxiway-sh/taxiway/blob/main/.agents/skills/taxiway-testing/SKILL.md#e2e-scenarios) says which
scenarios a change must run before merging. In a cloud session, read its
[environment notes](https://github.com/taxiway-sh/taxiway/blob/main/.agents/skills/taxiway-testing/references/cloud-environment.md)
first.

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
| `test_lima_startup.py` | The lifecycle E2E scenarios use Docker; Lima fresh readiness, boot-marker rejection/retry and restart are live recipes | The [`taxiway-testing` skill](https://github.com/taxiway-sh/taxiway/blob/main/.agents/skills/taxiway-testing/SKILL.md#live-validation-docker-or-lima) routes to bounded live recipes; do not claim automated Lima coverage |
| `test_claude_models.py` | Existing gateway/orchestrator E2Es cover protocol routing, model configuration, permission defaults and restart | Actual alias/full-ID access, inherited/explicit child models, tools/build/HTTPS, OAuth reuse after restart |
| `test_codex_models.py` | Existing gateway/orchestrator E2Es cover Responses routing, model configuration, permission defaults and restart | Actual principal/child model access, session relationships, gateway tokens and delegated tool effects |
| `test_gastown.py` | Existing Gas Town E2Es cover workspace/roles, model/alias/gateway environment, self-sling and handoff process/configuration preservation | Actual patrol checks/report/heartbeat progress, fresh startup, progress after Deacon handoff, launcher inference and delegation |
| `gateway_protocol_e2e_test.go` and its Python runner | Protocol assertions are direct guarantees of the existing lifecycle actions, using each lab's actual sidecar and controlled upstream, including its database/telemetry | No real-account claim |

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
`test_*.sh` files recursively. Under `set -o pipefail`, do not cut a pipe
short (`cmd | head` can fail with SIGPIPE); read the needed lines another way,
such as `sed -n` on a captured output.

### End-to-end

Session checks are Go helper functions in `internal/cli/orchestrator_e2e_test.go`.
They execute commands through the driver and assert their results directly.
Do not add a separate unit-test layer for E2E assertions.
Tests of the shipped launcher and profile remain under `tests/scripts/`.

Keep Lab lifecycle checks independent of orchestrator-specific expectations.
Agent checks follow the agents declared in the orchestrator manifest: the same
Claude Code trust assertion applies to both the Claude Code and Gas Town
orchestrators. Orchestrator expectations select the workspace paths and when
they should be trusted. Standalone agents use `/lab/work/e2e-fixture`; Gas
Town uses its rig's crew workspace, trusted when the agent is launched.
`/lab/work` is checked after installation for every declared agent supported
by these scenarios. Gas Town and Gas City session diagnostics remain separate
from the shared `shell --check` assertion.

The end-to-end suite exercises `claude-code`, `codex`, `gastown`, and `gascity` through
the Docker driver, using their real orchestrator and agent assets.

The scenarios cover installation and verification inside the lab, gateway
access, workspace preparation, start, stop, restart, diagnosis, reset, and
removal. They mirror the public `taxiway-sh/e2e-fixture` repository into a
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
| `ci.yml` | Push, pull request, or manual | `core-checks`: build, Go unit tests, shell tests, and lints; `site-checks`: site tests and build |
| `pages.yml` | Push to `main` that changes `site/`, `docs/` or the workflow itself, or manual | Site tests and build, then deployment of the site |
| `e2e.yml` | Daily at 22:17 UTC, or manual | Source-tree orchestrator lifecycles through Docker |

End-to-end tests run separately from the fast core checks. Their matrix uses
`fail-fast: false`, so one failure does not cancel the other orchestrators.

The end-to-end workflow has one Ubuntu job per orchestrator. Each job runs
`up`, `prepare-run`, and `phase-by-phase` in that order.

Run the site checks locally for documentation and frontend changes; the
`site-checks` job of `ci.yml` runs them again on every branch push and on pull
requests to `main`.

For end-to-end failures, inspect the failing orchestrator's `up`, `prepare-run`,
or `phase-by-phase` step, then rerun that scope locally.

## Adding or updating tests

The [`taxiway-testing` skill](https://github.com/taxiway-sh/taxiway/blob/main/.agents/skills/taxiway-testing/SKILL.md#change--checks) maps each kind of
change to its checks and says where its assertions go.

The existing Go E2E files start with the following directive and a blank line:

```go
//go:build e2e

```

Local Docker availability checks may use `requireDockerOrSkip(t)`.

When adding documentation pages, test routes, links, and navigation rather
than asserting a fixed total page count.
