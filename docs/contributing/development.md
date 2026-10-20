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

Use `make test-e2e-only` to run only end-to-end tests.
Use `make test-e2e-claude-code`, `make test-e2e-codex`, or
`make test-e2e-gastown` to run one orchestrator integration.
Use the `*-up` variants, followed by the `*-prepare-run` variants, before the
`*-phase-by-phase` variants when debugging the scheduled orchestrator end-to-end
sequence locally.
Use `make test-scripts` to run shell script tests.
`make lint` also runs `scripts/check-agent-docs.sh`, which validates skill
frontmatter and discovery symlinks, relative links in agent and contributor
docs, and that each normative phrase it lists appears only in its owner file.

See [Testing](testing.md) and [Drivers](../README.md#drivers).

## Contribution Workflow

The portable [Agent Skills](https://agentskills.io/) skill
[`taxiway-workflow`](https://github.com/taxiway-sh/taxiway/blob/main/.agents/skills/taxiway-workflow/SKILL.md)
is the session entry point and owns the contribution procedure: intake, backlog, approvals, coordination, independent
review, status, pause, resume and finish. This section only explains how to
start it. Codex discovers it in `.agents/skills/`; `.claude/skills/taxiway-workflow`
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

Set the `--max-agents` and `--max-heavy-tests` capacity hints if needed:

```bash
bash scripts/contribution-workflow.sh codex --max-agents 3 --max-heavy-tests 1
```

Use `--help` for settings and their defaults. Options after `--` are passed verbatim to the client,
for example `... codex -- --profile my-profile`.

To resume, restart with the launcher and request **resume**, or use your
client's native session-resume mechanism and invoke the same skill.

## Worktrees and Branches

Branch rules are in the [invariants](https://github.com/taxiway-sh/taxiway/blob/main/AGENTS.md#invariants).
Each subject has one worktree named `.worktrees/issue-<number>-<short-subject>`
and an intention-appropriate branch, for example `fix/93-gastown-deacon-startup` or
`feat/117-portable-contribution-workflow`. Start from current `main`. Inspect
`git worktree list` and existing branches first, and preserve existing work
rather than creating duplicate assignments.

## Pull Requests

A PR describes the final behavior, checks and limitations, links the issues it
resolves with `Fixes #...`, and uses appropriate existing labels from
[Issues](issues.md). Its title follows [Conventional Commits](#commit-messages).
The merge criterion of the [E2E scenario rule](https://github.com/taxiway-sh/taxiway/blob/main/AGENTS.md#e2e-scenarios)
applies; [Testing](testing.md) explains how to run the scenarios. Publishing and merging follow the skill's approvals.

## Upstream Dependency Bugs

For suspected LiteLLM, Langfuse, Claude Code, Codex, Gas Town or other dependency
bugs, first identify the installed version, supported current version and exact
integration boundary. Check existing reports and released fixes before deciding
whether Taxiway integration or upstream behavior is responsible. Find the
project's official contribution/reporting channel: do not assume every service
has a public source repository or accepts code contributions. Repository-specific
instructions take precedence within an upstream checkout.

Prepare a minimal, sanitized reproduction with observed/expected behavior,
versions, impact and relevant public evidence. Propose an upstream issue using
that project's template, or prepare a narrow patch in a separate owned checkout
with meaningful tests and a different review agent. Keep tokens, host details,
private logs and unrelated Taxiway changes out of both. Investigate enough to
make the proposal reviewable before requesting publication approval.

Authorization to work on Taxiway is not permission to fork an upstream project,
push code, open an issue/PR, or post comments there. Ask for explicit approval
of the concrete upstream destination and prepared content before each new
external contribution scope. Do not silently publish or modify upstream labels.
Authorized upstream `gh` commands must use the verified
`--repo <owner>/<repository>` explicitly, never the current-directory default or
Taxiway's target by mistake (see the [invariants](https://github.com/taxiway-sh/taxiway/blob/main/AGENTS.md#invariants)).
Track approved upstream links and status in the related Taxiway issue/PR, along
with any local workaround and the version/verification criteria for removing
it. Reuse an existing upstream report rather than opening duplicates.

## Post-Merge Cleanup

An authorized PR merge also authorizes cleanup of its development instance,
worktree, and feature branch; no separate confirmation is needed.

1. Coordinate with the worktree's agent and wait for its tests to finish.
   Check for uncommitted work and preserve anything not included in the PR.
2. Load that worktree's `.envrc` and identify its existing context id and lab
   state. Run `./taxiway list`, then `./taxiway destroy --yes` from that context
   to remove all its labs, gateways, observability, proxy, and generated state.
   Do this before removing the worktree or its runtime assets.
3. Verify that the context's Docker containers, networks, volumes, and Lima
   labs are gone. Investigate any leftovers by their exact context ownership;
   never use a global prune or remove another worktree's resources. If destroy
   or verification fails, keep the worktree for recovery and report the failure.
4. Remove the clean feature worktree and its local and remote feature branches
   after confirming the PR was merged. Do not discard uncommitted work or
   unrelated commits, and do not remove `main`.
5. Fast-forward local `main`, then update the other active feature branches
   against it. Coordinate rebases with their agents at a safe point so ongoing
   edits and tests are preserved. Resolve conflicts and rerun affected checks;
   force-pushing still requires explicit authorization.

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
