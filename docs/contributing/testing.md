# Testing

## Overview

Tests are split into two categories:

- **Unit tests** — no infrastructure dependency; run by default.
- **End-to-end tests** — exercise the Docker-backed orchestrator
  lifecycle with real runtime assets; gated by the `e2e` build tag.

End-to-end tests are reserved for system-level paths that are not well
represented by unit tests:

| Scope | Tests | What it proves | Requirement |
|---|---|---|---|
| Orchestrator | `TestE2E_Orchestrator_*` | Real orchestrator and agent assets can pass unauthenticated lifecycle flows. | `docker` on `PATH` and daemon |

## Conventions

| Category | Build tag | File suffix | Example |
|----------|-----------|-------------|---------|
| Unit | _(none)_ | `_test.go` | `docker_test.go` |
| End-to-end | `//go:build e2e` | `_e2e_test.go` | `orchestrator_e2e_test.go` |

Every end-to-end file **must** start with:

```go
//go:build e2e

```

(blank line after the directive is required by Go tooling.)

End-to-end files may also keep a `requireDockerOrSkip(t)` helper for local runs
where Docker is absent but the tag is active.

End-to-end test functions must use the `TestE2E_` prefix:

```bash
go test -tags=e2e -run '^TestE2E_' ./...
```

## Running tests locally

```bash
# Unit tests only (no Docker required)
make test-unit

# All Go tests with end-to-end files enabled
make test-e2e

# Only end-to-end tests
make test-e2e-only

# Only one orchestrator's end-to-end tests
make test-e2e-claude-code
make test-e2e-codex
make test-e2e-gastown

# Only up tests, across orchestrators
make test-e2e-up

# Only phase-by-phase tests, across orchestrators
make test-e2e-phase-by-phase

# Shell script tests
make test-scripts

```

`make test` is an alias for `make test-unit`.

## Command Reference

| Command | What it runs | Includes unit tests | Includes end-to-end tests |
|---|---|:---:|:---:|
| `make test-unit` | `go test ./...` | yes | no |
| `make test-e2e` | `go test -tags=e2e ./...` | yes | yes |
| `make test-e2e-only` | `go test -tags=e2e -run '^TestE2E_' ./...` | no | yes |
| `make test-e2e-up` | Aggregate `taxiway up` tests for all orchestrators | no | yes |
| `make test-e2e-prepare-run` | Aggregate prepare+run tests for all orchestrators | no | yes |
| `make test-e2e-phase-by-phase` | Phase-by-phase lifecycle tests for all orchestrators | no | yes |
| `make test-e2e-claude-code` | Claude Code orchestrator end-to-end tests | no | yes |
| `make test-e2e-claude-code-up` | Claude Code `taxiway up` end-to-end test | no | yes |
| `make test-e2e-claude-code-prepare-run` | Claude Code prepare+run end-to-end test | no | yes |
| `make test-e2e-claude-code-phase-by-phase` | Claude Code phase-by-phase end-to-end test | no | yes |
| `make test-e2e-codex` | Codex orchestrator end-to-end tests | no | yes |
| `make test-e2e-codex-up` | Codex `taxiway up` end-to-end test | no | yes |
| `make test-e2e-codex-prepare-run` | Codex prepare+run end-to-end test | no | yes |
| `make test-e2e-codex-phase-by-phase` | Codex phase-by-phase end-to-end test | no | yes |
| `make test-e2e-gastown` | Gas Town orchestrator end-to-end tests | no | yes |
| `make test-e2e-gastown-up` | Gas Town `taxiway up` end-to-end test | no | yes |
| `make test-e2e-gastown-prepare-run` | Gas Town prepare+run end-to-end test | no | yes |
| `make test-e2e-gastown-phase-by-phase` | Gas Town phase-by-phase end-to-end test | no | yes |
| `make test-scripts` | shell script tests under `tests/scripts/` | no | no |

Use `make test-e2e` for the Go end-to-end gate. Use
`make test-e2e-only` when you want only end-to-end test functions. Use the
orchestrator-specific targets when debugging one integration locally. Use the
`*-up` targets first, then the `*-prepare-run` targets, before running the
`*-phase-by-phase` targets.

E2E targets use `E2E_TIMEOUT` as their Go test timeout. Override it locally
when debugging slow Docker pulls or first-run setup:

```bash
E2E_TIMEOUT=1800s make test-e2e-gastown-up
```

## Shell Script Tests

Shell script tests validate small runtime shell contracts without creating a
lab or requiring Docker. They run through `make test-scripts`:

| Script | What it validates |
|---|---|
| `tests/scripts/test_standalone_install.sh` | Root `install.sh` defaults, help text, runtime directory, and staged runtime replacement. |
| `tests/scripts/test_verify_headers.sh` | Verify scripts start user-visible output with section headers before `OK` lines. |
| `tests/scripts/infra/commands/test_reset.sh` | `infra/commands/reset.sh` confirmation behavior, cleanup, and marker preservation. |
| `tests/scripts/infra/trace/test_events.sh` | `infra/trace/events.sh` event output, source fallback, and ignored trace env vars. |
| `tests/scripts/infra/workspace/test_clone.sh` | `infra/workspace/clone.sh` URL rewriting, clone flow, ref checkout, and token handling. |
| `tests/scripts/orchestrators/gastown/test_workspace.sh` | `orchestrators/gastown/workspace.sh` skip behavior, name validation, HQ initialization, rig registration, ref checkout, and crew workspace provisioning. |
| `tests/scripts/orchestrators/gastown/test_start.sh` | `orchestrators/gastown/start.sh` start phase behavior and contract validation. |

## End-to-end Scope

The end-to-end suite runs Taxiway through the Docker driver and real
orchestrator/agent asset layouts for `claude-code`, `codex`, and `gastown`.
It runs install and verify scripts for each orchestrator and its declared
agents, then verifies that each lab can reconcile gateway access, mirror the
public `manufacture-dev/agreement-hub` fixture repository into a lab-local bare
Git repository, let the orchestrator workspace scripts clone from that isolated
local remote, start, stop, restart, diagnose, reset, and remove a lab. After
start and restart, the suite also runs
`taxiway shell <lab> --check` to verify that the public shell command can
resolve a ready target without opening an interactive session.

Separate Docker-driver, fixture-orchestrator, and installed-runtime-layout
end-to-end tests are redundant for this lifecycle path; driver details,
runtime path resolution, gateway handling, and phase edge cases are covered by
unit tests, including Docker driver command construction against a fake `docker`
binary.

These tests intentionally pass `--skip-auth-check`, do not run the interactive
`taxiway auth` command, do not use real API keys, and do not exercise browser
or device-flow login. Authenticated agent execution is outside this layer
because it depends on external accounts, quotas, opaque local credential
stores, and interactive state.

Docker-backed end-to-end tests skip cleanly when Docker is unavailable. Set
`LAB_NO_DOCKER=1` to force the skip path:

```bash
LAB_NO_DOCKER=1 make test-e2e-only
```

The Docker driver details, prerequisites, and troubleshooting notes live in
[Docker](../drivers/docker.md).

## CI Structure

`core-checks` runs on every push and pull request. It covers build, unit tests,
shell script tests, and lints.

| Job | What it runs | Docker daemon |
|-----|--------------|---------------|
| `core-checks` | `go build` + `make test-unit` + `make test-scripts` + lints | No |

The Docker-backed orchestrator end-to-end suite is intentionally not a PR
merge gate because real installs make it too slow and sometimes dependent on
external package availability. It runs from the `End-to-end` workflow on
a daily schedule and through `workflow_dispatch`. That workflow runs a matrix
with one job per orchestrator. Each job runs the `taxiway up` test for its orchestrator first, then runs the
corresponding `prepare-run` test, and finally runs that orchestrator's
phase-by-phase test. The orchestrator jobs run in parallel with `fail-fast` disabled.

## Adding a new test

**Unit test** — write it in the nearest `_test.go` file (no build tag needed).
Verify it compiles and passes with `make test-unit`.

**End-to-end test** — create or extend a `*_e2e_test.go` file in the same package.
Add `//go:build e2e` as the very first line, followed by a blank line.
Name the test function `TestE2E_<area>_<case>`. Verify with
`make test-e2e-only` and `make test-e2e`.
