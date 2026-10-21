---
name: taxiway-testing
description: Use when choosing the checks a Taxiway change needs, adding or extending E2E scenarios (including for a new orchestrator), choosing Docker or Lima for live validation, deciding whether installation qualification must change, or running tests in a cloud session.
---

# Taxiway testing

Owns which checks a change needs and where its durable assertions go. Read
[`AGENTS.md`](../../../AGENTS.md) first. The commands live in
[Testing](../../../docs/contributing/testing.md),
[Live validation](../../../docs/contributing/live-tests.md),
[`tests/live/AGENTS.md`](../../../tests/live/AGENTS.md) and
[Installation qualification](../../../docs/contributing/installation-qualification.md).

## Change → checks

Run every row the change touches, on its current head, and record each result
for the PR's Validation section. `make lint` applies to every change.

| Change | Checks |
|---|---|
| Go behavior, driver command construction, configuration | Unit: nearest `_test.go` without build tag; `make test-unit` |
| Shell script: installer, phase, runtime or orchestrator script | Scripts: a `test_*.sh` under `tests/scripts/`; `make test-scripts` |
| Repository tool under `scripts/` or `tests/live/` | Its own test under `tests/scripts/` (for example `tests/scripts/live/test_helpers.sh`); release evidence tools: `make test-release-tools` |
| `AGENTS.md`, skills, scheduled agents, contributor docs | Lint: `make lint-agent-docs` and `bash tests/scripts/test_check_agent_docs.sh` |
| Pages under `docs/` or the site | Site: `npm test` and `npm run build` in `site/` |
| What a lab receives or does: orchestrator adapter, agent assets, gateway, driver, lifecycle phases | Per-orchestrator E2E: the affected [scenarios](#e2e-scenarios) on Docker |
| Behavior a person would check by hand in a lab | Live validation on [Docker or Lima](#live-validation-docker-or-lima), provider-free when possible |
| Real model, account, delegation or patrol behavior | Live validation with a reference login; real-provider checks need approval ([workflow](../taxiway-workflow/SKILL.md#approvals)) |
| Installer, release assets, `taxiway init`, host prerequisites, guest runtime contract | [Installation qualification](#installation-qualification) |

Test product behavior directly: no separate test layer for E2E assertions, no
committed feature-specific live suite. A process being alive, a successful exit
or a gateway health check does not prove configuration, model selection or the
effect of an agent action; assert observable results. Keep expected values
independent of the production code that selects or renders them, and include
restart and handoff paths when the behavior must survive session renewal.
Real-provider live validation adds evidence; it never replaces the
credential-free E2E assertions.

## E2E scenarios

Decided in [#177](https://github.com/taxiway-sh/taxiway/issues/177).

- Each orchestrator has three lifecycle E2E scenarios on Docker: Up,
  PrepareRun and PhaseByPhase, in `internal/cli/orchestrator_e2e_test.go`.
- When a change affects provisioned or runtime behavior, enrich the affected
  scenarios with assertions and helpers in that file, written as
  [Writing E2E scenarios](../../../docs/contributing/testing.md#writing-e2e-scenarios)
  says. Do not add parallel suites, scenario files, `TestE2E_` entry points,
  jobs or runners; `e2e_support_test.go` is setup and cleanup only.
- Adding an orchestrator adds its own three scenarios, its
  `test-e2e-<orchestrator>` Makefile targets (with `-up`, `-prepare-run` and
  `-phase-by-phase`) and its `e2e.yml` matrix entry, like the existing
  orchestrators. This follows the rule; it needs no exception.
- Run the scenarios of every orchestrator that consumes the change (an agent
  shared by several orchestrators covers each of them), and all three when the
  Up, PrepareRun and PhaseByPhase paths are all affected.
- Merge criterion: the affected scenarios ran and passed on the proposed code;
  an external contributor's run on their own machine is evidence only and does
  not meet it. Core CI does not run them; a skipped or blocked run is not validation:
  report it and resolve it before claiming E2E validation.

## Live validation: Docker or Lima

Live validation is the agent testing its feature in a real lab, as a person
would. Choose the driver from the feature:

- **Docker** by default: orchestrator, agent, gateway, recording and other
  behavior shared by both drivers; it is also the E2E driver.
- **Lima** when the change touches the Lima driver, VM boot and readiness,
  systemd user units, mounts, port forwards, or a behavior documented as
  differing in [`docs/drivers/`](../../../docs/drivers/). Docker results do not
  qualify Lima.
- **Both** when one behavior is expected on both drivers but the code path
  differs by guest kind.

Prefer provider-free checks (prepared labs, controlled fixtures). When one
fails, use the live helper's diagnostic output
([`tests/live/AGENTS.md`](../../../tests/live/AGENTS.md)) instead of bypassing
the helper with raw `docker exec` or `limactl shell`.

A cloud session cannot validate Lima (no KVM or Lima), a macOS host, or
real-account behavior (no maintainer login). List those as pending as
[`steward`](../steward/SKILL.md#ci) says; never infer them from Docker results.

## Installation qualification

`install.yml` installs a published release on each platform × driver runner
and checks lab creation. Extend it, in the same change, when the change alters
what a user installs or needs before the first lab: the installer, release
archive layout or runtime assets, `taxiway init`, host prerequisites (Docker,
Compose, Lima versions), supported runner platforms, or the guest runtime
contract checked after `create`.

- Extend the existing `linux`, `macos` and `windows` jobs, their driver matrix
  and their common steps; no new workflow.
- A check for behavior not yet released is gated on the first release that
  ships it and logs an explicit skip for older releases, as the guest runtime
  contract does.
- Update [Installation qualification](../../../docs/contributing/installation-qualification.md)
  when the scenario or runner coverage changes. Running the workflow is a
  GitHub action under the [approvals](../taxiway-workflow/SKILL.md#approvals).

## Cloud sessions

Before running Docker, E2E or Go tests in a cloud container, read
[`references/cloud-environment.md`](references/cloud-environment.md): the
Docker daemon, the root user, the network proxy and GitHub API access differ
from a workstation, and ignoring them invalidates runs.
