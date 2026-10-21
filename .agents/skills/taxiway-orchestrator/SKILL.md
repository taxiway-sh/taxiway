---
name: taxiway-orchestrator
description: Use when adding a new orchestrator adapter to Taxiway or reworking how an existing one installs, runs, wires the gateway or works around an upstream bug, including the feasibility study, the file checklist and the installation principles per environment type.
---

# Adding an orchestrator

Owns the procedure for adding an orchestrator adapter, the checklist of files
it touches and the installation principles adapters follow per environment
type. Read [`AGENTS.md`](../../../AGENTS.md) first: its invariants (adapter
boundary, one responsibility per script, one behavior across contexts) apply
throughout. The adapter contract itself is the
[Adapter Boundary](../../../docs/reference/architecture.md#adapter-boundary);
the reference addition is Gas City
([#176](https://github.com/taxiway-sh/taxiway/issues/176),
[#185](https://github.com/taxiway-sh/taxiway/pull/185)).

## Recipe

1. **Feasibility.** Map the orchestrator onto the
   [adapter contract](../../../docs/reference/architecture.md#adapter-boundary)
   (manifest, phase scripts, optional `stop.sh`, supported agents) and research
   upstream: official install and runtime method per environment type
   ([principles](#installation-principles)), releases and published checksums
   to pin (Gas City pins an exact release), non-interactive initialization,
   how it runs its daemons and sessions, where it lets you configure the model gateway
   ([gateway wiring](#gateway-wiring)), and known bugs at the pinned version.
   Name what the contract cannot express yet instead of planning a Go branch.
2. **Issue.** Propose it through [`taxiway-intake`](../taxiway-intake/SKILL.md)
   with the feasibility findings and open decisions.
3. **Spike.** Try the install, initialization and start by hand in a throwaway
   Docker lab without a real provider and without commits. Summarize the
   findings (commands, versions, surprises) for the issue under the
   [approvals](../taxiway-workflow/SKILL.md#approvals), then destroy the lab.
4. **Decisions.** Turn each open choice (pinned versions, templates, settings
   and defaults, supervisor model per guest, limitations to accept) into a
   question for the maintainer, asked as
   [Interaction](../taxiway-workflow/SKILL.md#interaction) says, and record the
   answers as [Decisions](../taxiway-workflow/SKILL.md#decisions) says.
5. **Implementation.** Follow [`taxiway-issue`](../taxiway-issue/SKILL.md),
   covering every [checklist](#file-checklist) entry. Test selection and E2E
   scope belong to [`taxiway-testing`](../taxiway-testing/SKILL.md#change--checks).
6. **Review.** Follow [`taxiway-review`](../taxiway-review/SKILL.md).
7. **PR.** Open it as [`taxiway-issue`](../taxiway-issue/SKILL.md#6-pull-request)
   says and follow it with [`steward`](../steward/SKILL.md); the PR lists the
   accepted limitations and their tracking issues.

## File checklist

Derived from the categories #185 touched. `<name>` is the orchestrator type.

**Adapter**

- `orchestrators/<name>/manifest.yaml`: name, description, `docs_url`,
  supported agents and settings (defaults, examples, phases).
- `orchestrators/<name>/install.sh`, `verify.sh`, `workspace.sh`, `start.sh`,
  `doctor.sh`, and `stop.sh` when the adapter starts services; shared helpers
  in `orchestrators/<name>/lib.sh`.
- `infra/commands/reset.sh` names no orchestrator: stopping belongs in
  `stop.sh`, which `taxiway reset` runs first.

**Go touch points** (debt tracked in
[#181](https://github.com/taxiway-sh/taxiway/issues/181); add nothing beyond
these)

- `internal/cli/port_forward.go`: the guest port of a dashboard or API to
  forward (`orchestratorPortForwards`), with `port_forward_test.go`.
- `internal/cli/labref.go`: the orchestrator types listed in help text.
- `internal/cli/up.go` (`buildBaseEnv`): only if the orchestrator needs
  workspace variables the shared environment lacks; prefer the manifest and
  propose the missing contract field under #181.
- `internal/discover/discover_test.go`: discovery of the new manifest.

**Shared changes** (not part of the adapter; each is its own commit and serves
every adapter)

- *Contract extension*, such as the `stop.sh` hook #185 added: the script
  lookup in `internal/config/config.go` (`StopScript`), the `internal/cli`
  command that calls the hook (`aliases.go` for `reset`) and its tests
  (`aliases_test.go`), the shared script tests under
  `tests/scripts/infra/commands/` (`test_reset.sh`), and the hook in existing
  adapters when they need it, with its test (`orchestrators/gastown/stop.sh`,
  `tests/scripts/orchestrators/gastown/test_stop.sh`).
- *Guest runtime change*, such as `Driver.GuestKind()` and
  `TAXIWAY_GUEST_KIND`: the drivers (`internal/driver/driver.go`,
  `docker.go`, `lima.go`, `mock.go`, `dryrun.go`) and their tests
  (`driver_test.go`, `docker_test.go`), the environment passed to scripts in
  `internal/cli/labops.go` and `dryrun_plan.go` (`withGuestKind`),
  `internal/cli/aliases.go` (bootstrap environment), `internal/cli/up_test.go`,
  and `infra/commands/bootstrap.sh`.

**Tests**

- `tests/scripts/orchestrators/<name>/test_*.sh` for each script with logic,
  plus the adapter's entries in `tests/scripts/test_dry_run_plans.sh` and
  `tests/scripts/test_verify_headers.sh`.
- The scenarios, Makefile targets and CI entries the
  [E2E scenario rule](../taxiway-testing/SKILL.md#e2e-scenarios) requires, in
  `internal/cli/orchestrator_e2e_test.go` and `Makefile` (`.PHONY`, the
  per-scenario targets and a timeout variable when the default is too short).
- `.github/workflows/e2e.yml` and `.github/workflows/release.yml`: the
  orchestrator in the matrix, and the job `timeout-minutes` when its scenarios
  run longer.
- `tests/release/qualify.py`: the `e2e-<name>` and `live-<name>` items.
- `.github/workflows/install.yml`: only when the guest runtime contract
  changes (#185 added its checks after `taxiway create`).

**Documentation**

- `docs/orchestrators/<name>.md`, with the sections of
  `docs/orchestrators/gascity.md` that apply, including where the gateway key
  appears ([gateway wiring](#gateway-wiring)) and the runtime per environment
  type.
- Orchestrator lists and tables: `README.md`, `docs/README.md`,
  `docs/reference/concepts.md`; `docs/reference/architecture.md` only when the
  adapter contract or guest runtime changes.
- Contributor docs: `docs/contributing/testing.md` (targets, timeouts,
  coverage), `docs/contributing/development.md` (E2E targets),
  `docs/contributing/release-qualification.md` (evidence items),
  `docs/contributing/installation-qualification.md` and `docs/drivers/` only
  for a guest runtime change.

**Landing page and docs site**

- `site/src/docs/loader.js`: register `orchestrators/<name>` so the docs page
  renders.
- `site/src/sections/Hero.jsx`, `CtaSection.jsx`, `FeatureGrid.jsx` and
  `ConsolePreview.jsx`: the orchestrator's name and docs link wherever the
  others appear; `site/src/__tests__/content.test.jsx` asserts it.

Before the PR, search the tree for an existing orchestrator's name
(`git grep -il gastown`) and confirm each hit has a counterpart or a reason
not to.

## Installation principles

This section is the only statement of these principles. The mechanics adapters
rely on (the `TAXIWAY_GUEST_KIND` value, PID 1, systemd availability) are the
[guest runtime contract](../../../docs/reference/architecture.md#adapter-boundary).
Decided in [#179](https://github.com/taxiway-sh/taxiway/issues/179), which
stays open for the remaining product work (process reaping checks, removing
the Docker `systemctl` stub with the engine install, container engine as a
lab capability).

- **Official method per environment type.** Install and run each tool the way
  it officially supports for that environment: its container guidance in
  Docker labs, its normal VM or host path in Lima labs. Identical installs
  across drivers are not a goal; comparable, documented behavior is, and the
  adapter page lists the differences.
- **Docker labs are first-class**, not a test-only path; see the
  [invariants](../../../AGENTS.md#invariants) for behavior across drivers.
- **Install what the lab needs without weakening isolation.** Never mount the
  host Docker socket or widen container privileges to make a tool work.
- **No silent shims.** A tool probing its environment (for example
  `systemctl --user`) gets a truthful answer; do not fake a service manager or
  binary to steer it onto a path.
- **Heavy capabilities are opt-in.** A container engine, large templates or
  other costly components are explicit settings or lab capabilities, off by
  default.
- **Same behavior on every host OS**: macOS, Linux and Windows with WSL2; see
  the [invariants](../../../AGENTS.md#invariants).
- **Taxiway intervenes only for its own concerns.** Gateway, authentication,
  trust, recording, autonomy and the lab lifecycle (start, stop, reset);
  process management stays the orchestrator's.

## Gateway wiring

- Route every agent through the lab's LiteLLM gateway with the
  orchestrator's **native** upstream or provider configuration (Gas City: an
  `[upstreams]` entry in a Taxiway-managed `taxiway.toml`), rewritten by each
  `start` so restarts are idempotent.
- The gateway key lives only in files of mode `0600` (the lab environment file
  `~/.config/taxiway/env`, or an adapter-managed config file). Configuration
  references `$TAXIWAY_LITELLM_API_KEY` rather than the value where the
  orchestrator supports it.
- Document in the adapter page every place the key appears (files, units,
  process arguments, tmux environment) and compare with the other adapters:
  Claude Code, Codex and Gas City pass it to their tmux sessions with
  `tmux new-session -e`, so it briefly appears in process arguments; Gas Town
  sources the lab environment file in `launch-agent.sh`; Gas City also writes
  it into a Lima supervisor unit
  ([Gateway key exposure](../../../docs/orchestrators/gascity.md#gateway-key-exposure),
  tracked in [#180](https://github.com/taxiway-sh/taxiway/issues/180)).
  Gateway-key and secrets handling beyond this belongs to
  [`taxiway-security`](../taxiway-security/SKILL.md#gateway-keys-and-lab-isolation).

## Upstream workarounds

A bug in the pinned version follows
[`taxiway-upstream`](../taxiway-upstream/SKILL.md#suspected-upstream-bug);
a workaround Taxiway keeps follows its
[Workaround and tracking issue](../taxiway-upstream/SKILL.md#workaround-and-tracking-issue).
In an adapter, the code comment sits in the adapter script that carries the
workaround (example: `orchestrators/gascity/start.sh` cites
gastownhall/gascity#5985 and #178), and the documented limitation goes in the
Limitations section of `docs/orchestrators/<name>.md`.

## Keeping this skill current

When [#181](https://github.com/taxiway-sh/taxiway/issues/181) moves port
forwards, workspace variables and shell targets into manifests and removes
orchestrator names from Go, update the Go touch points and the manifest entry
of the [checklist](#file-checklist) in the same PR. Update the checklist
whenever an orchestrator addition touches a file category it does not list.
