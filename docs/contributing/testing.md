# Testing

Taxiway's development tests cover individual behaviors, runtime scripts,
orchestrator lifecycles, and the documentation site. Choose the suite that
matches your change.

To verify a published release across platforms and drivers, see
[Installation qualification](installation-qualification.md).

## Choosing tests

| Suite | What it checks | What you need |
|---|---|---|
| Unit | Go behavior, driver commands, configuration, and phase edge cases | Go; no Docker or Lima |
| Shell scripts | Installer and runtime script contracts | Local shell tools; no running lab |
| End-to-end | Orchestrator lifecycle using source-tree runtime assets | Go and a running Docker daemon |
| Site | Documentation routing, navigation, rendering, and landing page content | Node.js and the site dependencies |

Unit and shell tests provide quick feedback during development. End-to-end
tests cover what happens when an orchestrator is provisioned and operated
inside a lab.

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

The end-to-end suite exercises `claude-code`, `codex`, and `gastown` through
the Docker driver, using their real orchestrator and agent assets.

The scenarios cover installation and verification inside the lab, gateway
access, workspace preparation, start, stop, restart, diagnosis, reset, and
removal. They mirror the public `manufacture-dev/agreement-hub` fixture into a
lab-local bare Git repository, then clone the working tree from that isolated
remote. After start and restart, `taxiway shell <lab> --check` verifies that
the session target is ready without opening an interactive shell.

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

| Change | Where to add coverage |
|---|---|
| Go behavior or driver command | Nearest `_test.go` file, with no build tag; verify with `make test-unit` |
| Shell behavior | A `test_*.sh` file under `tests/scripts/`; verify with `make test-scripts` |
| Orchestrator lifecycle | A `*_e2e_test.go` file; verify with the relevant end-to-end target |
| Documentation page or site navigation | Existing tests under `site/src/`; run site tests and build |

End-to-end files must start with the following directive and a blank line:

```go
//go:build e2e

```

Use the `TestE2E_` prefix for end-to-end functions so the focused targets find
them. Local Docker availability checks may use `requireDockerOrSkip(t)`.

When adding documentation pages, test routes, links, and navigation rather
than asserting a fixed total page count.
