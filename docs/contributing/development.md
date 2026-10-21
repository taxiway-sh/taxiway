# Development

Local development requires [Go](https://go.dev/doc/install).

## Source Checkout Runtime

Load `.envrc` when running Taxiway from the checkout:

```bash
source .envrc
```

It sets:

```bash
export TAXIWAY_CONTEXT=dev
export TAXIWAY_CONTEXT_ID="$(test -f "$PWD/.context-id" || uuidgen | tr '[:upper:]' '[:lower:]' | cut -c1-8 > "$PWD/.context-id"; cat "$PWD/.context-id")"
export TAXIWAY_AUTH_DIR="$PWD/.auth"
export TAXIWAY_OBSERVABILITY_DIR="$PWD/.observability"
export TAXIWAY_PROXY_DIR="$PWD/.proxy"
export TAXIWAY_RUNTIME_DIR="$PWD"
export TAXIWAY_LAB_STATE_DIR="$PWD/.lab-state"
```

These values make `./taxiway` and `go run ./cmd/taxiway` use runtime assets
from the checkout, keep lab state in `./.lab-state`, and create an isolated
dev runtime under `./.auth`, `./.observability`, and `./.proxy`. The context id
is generated once into `./.context-id` so two local checkouts do not reuse the
same Docker Compose project or gateway container, and so moving the checkout
does not change the environment id.

Optional: use `direnv` to load `.envrc` automatically.

## Build

```bash
go build -o ./taxiway ./cmd/taxiway
./taxiway version
```

The Makefile target builds all packages:

```bash
make build
```

## Test

```bash
make test-unit
make lint
```

Docker-backed end-to-end tests require Docker and are intended for local,
scheduled, or manually triggered validation:

```bash
make test-e2e
```

Use `make test-e2e-only` to run only end-to-end tests. The aggregate targets
run every orchestrator in sequence (about 2.5 hours, `E2E_SUITE_TIMEOUT`); see
[Testing](testing.md) for the per-target timeouts.
Use `make test-e2e-claude-code`, `make test-e2e-codex`, `make test-e2e-gastown`, or
`make test-e2e-gascity` to run one orchestrator integration.
Use the `*-up` variants, followed by the `*-prepare-run` variants, before the
`*-phase-by-phase` variants when debugging the scheduled orchestrator end-to-end
sequence locally.
Use `make test-scripts` to run shell script tests.
`make lint` also runs `scripts/check-agent-docs.sh`, which validates skill
frontmatter and discovery symlinks, relative links in agent docs and in every
page under `docs/` (which reach files outside it through blob URLs), and that
each normative phrase it lists appears only in its owner file.

See [Testing](testing.md) and [Drivers](../README.md#drivers).

## Contribution Workflow

The portable [Agent Skills](https://agentskills.io/) skill
[`taxiway-workflow`](https://github.com/taxiway-sh/taxiway/blob/main/.agents/skills/taxiway-workflow/SKILL.md)
is the session entry point and owns the contribution procedure: routing, approvals, coordination,
status, pause, resume and finish. It routes new needs to
[`taxiway-intake`](https://github.com/taxiway-sh/taxiway/blob/main/.agents/skills/taxiway-intake/SKILL.md)
and backlog suggestions to
[`taxiway-triage`](https://github.com/taxiway-sh/taxiway/blob/main/.agents/skills/taxiway-triage/SKILL.md).
This section only explains how to start it. Codex discovers it in `.agents/skills/`; `.claude/skills/taxiway-workflow`
is a relative symlink to that same directory for Claude Code discovery. There
are no separate workflow implementations, Dynamic Workflow dependencies, or
client-specific schedulers.

### Start a session

Install the selected client, `git`, `gh`, and the tools required by the chosen
tests. Use the environment's existing authentication; the launcher never logs
in, modifies client settings, or changes sandbox/permission defaults. Invoke
the launcher from this checkout (or by its absolute path from any directory):

```bash
bash scripts/contribution-workflow.sh codex
bash scripts/contribution-workflow.sh claude
```

Both commands open a new interactive session in the checkout with the same
initial prompt, explicitly requesting the canonical skill and `AGENTS.md`. An
ordinary client session does not automatically start the workflow. In an
existing session, invoke `$taxiway-workflow` in Codex or `/taxiway-workflow` in
Claude Code, or explicitly ask it to read the canonical skill. That explicit
read also works if client skill discovery or Claude's `agents-md` plugin is
unavailable.

By default the session sizes agent concurrency to measured host capacity. Use
`--max-agents` to cap it and `--max-heavy-tests` to limit concurrent Docker/Lima runs:

```bash
bash scripts/contribution-workflow.sh codex --max-agents 3 --max-heavy-tests 1
```

Use `--help` for settings and their defaults. Options after `--` are passed verbatim to the client,
for example `... codex -- --profile my-profile`.

To resume, restart with the launcher and request **resume**, or use your
client's native session-resume mechanism and invoke the same skill.

## Issues and Pull Requests

Each selected issue is worked in its own worktree and branch, then through a
pull request, as described in the
[`taxiway-issue`](https://github.com/taxiway-sh/taxiway/blob/main/.agents/skills/taxiway-issue/SKILL.md)
skill, which also owns
[post-merge cleanup](https://github.com/taxiway-sh/taxiway/blob/main/.agents/skills/taxiway-issue/SKILL.md#post-merge-cleanup).
Independent review follows
[`taxiway-review`](https://github.com/taxiway-sh/taxiway/blob/main/.agents/skills/taxiway-review/SKILL.md),
and PR follow-up (CI, E2E results, conflicts, stacked PRs) follows
[`steward`](https://github.com/taxiway-sh/taxiway/blob/main/.agents/skills/steward/SKILL.md).
GitHub pre-fills new PR descriptions with the
[PR template](https://github.com/taxiway-sh/taxiway/blob/main/.github/pull_request_template.md).

## Upstream Dependency Bugs

Suspected bugs in LiteLLM, Langfuse, Claude Code, Codex, Gas Town, Gas City or
another dependency, upstream reports and patches, local workarounds and their
tracking issues, and dependency bumps follow the
[`taxiway-upstream`](https://github.com/taxiway-sh/taxiway/blob/main/.agents/skills/taxiway-upstream/SKILL.md)
skill.

## Commit Messages

Release notes are generated by GoReleaser from commit subjects, so commits must
follow [Conventional Commits](https://www.conventionalcommits.org/):

```text
<type>[(scope)][!]: <description>
```

Examples:

```text
feat: add a new capability
feat(record): support explicit window sizing
fix: correct broken behavior
cleanup: remove obsolete behavior
docs: update documentation
```

### What appears in the release notes

GoReleaser sorts each commit into a section by matching its subject, in this
order (the first match wins):

| Section | Matched subjects |
|---|---|
| **Features** | `feat:`, `feat(scope):`, `feat!:` |
| **Fixes** | `fix:`, `fix(scope):`, `fix!:` |
| **Cleanup** | any subject containing `cleanup`, `remove`, `simplif`, or `polish` (e.g. `cleanup:`) |

A `feat:`/`fix:` prefix takes precedence over the Cleanup keywords, so
`fix: remove race` lands under Fixes.

The following are **excluded** from the release notes — use them for changes
that should not appear in a release: `docs:`, `test:` / `tests:`, `chore:`,
`style:`, `refactor:`, merge commits, and reverts.

A commit matching no section and no exclusion is not grouped, so always use one
of the types above.

If a pull request is squash-merged, the squash commit subject (the PR title by
default) is what GoReleaser reads — give the PR a Conventional Commits title.

## Local Snapshot

To test the same packaging path as a release, build a GoReleaser snapshot and
copy the platform binary back into the repository:

```bash
make snapshot
```

Snapshot builds are local-only and include the current commit in the version,
for example `0.1.0-SNAPSHOT-2f1183a`.

## Local Completion

Generate completion for the local development binary:

```bash
make completion
```

You can also target a shell explicitly:

```bash
make completion-zsh
make completion-bash
make completion-fish
```

## Release

See [Release](release.md).
