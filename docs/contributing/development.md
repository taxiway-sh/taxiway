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

See [Testing](testing.md) and [Drivers](../README.md#drivers).

## Branches and Pull Requests

Never push directly to `main`. All changes, including documentation and agent
instructions, must be committed on a feature branch and integrated through a
pull request. Permission to publish a branch or merge a PR does not authorize a
direct push to `main`.

## Contribution Workflow

The portable contribution [Agent Skills](https://agentskills.io/)
skill is: [.agents/skills/taxiway-workflow/SKILL.md](https://github.com/taxiway-sh/taxiway/blob/main/.agents/skills/taxiway-workflow/SKILL.md).
Codex discovers it there; `.claude/skills/taxiway-workflow` is a relative symlink
to that same directory for Claude Code discovery. There are no separate workflow
implementations, Dynamic Workflow dependencies, or client-specific schedulers.
`AGENTS.md` owns policy; this page owns practical procedure.

### Start a session

Install the selected client, `git`, `gh`, and the tools required by the chosen
tests. Use the environment's existing authentication; the launcher never logs
in, modifies client settings, or changes sandbox/permission defaults. Invoke
the launcher from this checkout (or by its absolute path from any directory):

```bash
bash scripts/contribution-workflow.sh codex
bash scripts/contribution-workflow.sh claude
```

Both commands open a new interactive session with the same initial prompt,
explicitly requesting the canonical skill and `AGENTS.md`. An ordinary client
session does not automatically start the workflow. In an existing session,
invoke `$taxiway-workflow` in Codex or `/taxiway-workflow` in Claude Code, or
explicitly ask it to read the canonical skill. That explicit read also works
if client skill discovery or Claude's `agents-md` plugin is unavailable.

The first question is whether you have a new need/problem or want backlog
suggestions. For a new need, the coordinator investigates, checks duplicates,
and proposes issue scopes and acceptance criteria; issue creation needs your
authorization. For backlog suggestions, it ranks a short list by explicit
`priority:p0/p1/p2`, then project value (correctness, reproducibility,
observability, resetability), dependencies, readiness and capacity. It identifies
missing priorities and existing assignments without rewriting labels. Selecting
or resuming a named subject answers intake; it does not authorize publishing,
merging, or starting additional subjects.

Configure the active-session targets and capacity if needed:

```bash
bash scripts/contribution-workflow.sh codex --status-interval 300 --triage-interval 900 --max-agents 2 --max-heavy-tests 1
```

Use `--help` for settings. Options after `--` are passed verbatim to the client,
for example `... codex -- --profile my-profile`. The defaults count the
coordinator among agents; real client and machine capacity can reduce them.
Missing delegation prevents an independent review: report that limitation and
arrange a different reviewer instead of silently self-reviewing.

### Choose, implement and review

Before starting, inspect `git worktree list`, existing branches, issues and PRs.
Use `gh ... --repo taxiway-sh/taxiway` with bounded fields and results. Reconcile
actual running agents/tests with their owners; historical notes alone do not
prove a process is still running or a check passed on the current commit.
Read-only reconciliation does not need fresh user approval.

Each subject has one implementer, a separate reviewer, and a worktree named
`.worktrees/issue-<number>-<short-subject>`. Use an intention-appropriate branch,
for example `fix/93-gastown-deacon-startup` or `feat/117-portable-contribution-workflow`.
Start from current `main`; preserve existing work rather than creating duplicate
assignments. Make commits by intention. Beta features may break older behavior:
do not add historical migration/compatibility code or tests unless requested.
Current lifecycle, reset/recovery and idempotency tests remain relevant.

Follow [Testing](testing.md) and [Live Tests](live-tests.md), with real provider
tests where needed. Enrich affected existing E2Es rather than adding an
assertion-only layer or separate jobs. Coordinate Docker/Lima runs across
subjects: default to one heavy run at a time, identify context ownership, and
defer work if memory, network pools or client capacity are exhausted. Diagnose
and retry narrowly; never globally prune resources or delete another context.

Review the exact commit/diff with an agent other than the implementer. Resolve
findings, verify corrections, and re-review materially changed code. Then
prepare a PR describing final behavior, checks and limitations, with
`Fixes #...` and appropriate existing labels from [Issues](issues.md). Preparing
a PR does not authorize its publication or merge. Pending approval blocks that
action on that subject; it does not stop other already-authorized independent
implementation, tests, reviews or status reports. Check assignments before
moving to the next subject and respect capacity and dependencies. Do not end
an active session solely to wait for publication approval. If no independent
action remains, report the pending decisions and whether the client can remain
active; an external runner is not supplied. User authorization is required
for those actions; affected E2Es must be green on the candidate commit.

If implementation exposes an unrelated bug or improvement, describe sanitized
evidence, impact and suggested scope, check duplicates, and propose an issue.
Ask whether to create it unless already authorized. Keep acceptance-critical
fixes in the current subject and avoid silently expanding scope.

### Upstream dependency bugs

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
Taxiway `gh` commands keep `--repo taxiway-sh/taxiway`; authorized upstream `gh`
commands must use the verified `--repo <owner>/<repository>` explicitly, never
the current-directory default or Taxiway's target by mistake.
Track approved upstream links and status in the related Taxiway issue/PR, along
with any local workaround and the version/verification criteria for removing
it. Reuse an existing upstream report rather than opening duplicates.

### Monitoring, authentication and backlog

The coordinator maintains three independent loops during the active session.
Each has an owner and next due time; reports share pending questions so a single
blocker produces one actionable request. These are cooperative session loops,
not background services. Check due times at bounded task/wait boundaries, prefer
asynchronous tools for long-running work, and keep waits shorter than the status
interval. A blocked client call or a client unable to schedule turns may delay
an update: disclose that limitation rather than promise exact timers. Nothing
continues after the session exits without an external runner (not supplied).

| Loop | Trigger and default cadence | Output and boundary |
|---|---|---|
| Overall status | Initial intake, then every 300 seconds; milestones/blockers promptly | Every subject's stage, evidence, changes, waits, user action and next step; include unchanged-but-running work and next expected update. Never invent percentages or finish dates. |
| Live authentication | Before authenticated scenarios and on native auth rejection; no tight polling | One exact native reconnection request per confirmed reference blocker, bounded real verification, then authorized test-lab propagation. Other subjects continue. |
| Backlog | Intake/backlog request, then every 900 seconds | Refresh a short justified shortlist against priority, dependencies and active assignments. Coalesce unchanged recommendations; never start unselected work. |

Use `tests/live/taxiway_live.py` and `tests/live/AGENTS.md` for auth/lab handling.
File presence alone does not prove a usable token. Confirm an auth rejection
from sanitized native output; gateway timeout, network and model errors need
their own diagnosis. Follow [the reference-login commands](live-tests.md#authenticate-once)
in the affected context and give the user the exact command for their driver
and reference lab. If native refresh cannot recover, ask for that login once;
verify a bounded real request before copying to authorized test labs. Never
print tokens or alter host authentication. The existing reference preflight
runs a bounded native Claude request and permits native OAuth refresh before
copying authentication. Reuse it; this workflow does not implement another
OAuth client or infer validity from file presence.

For an existing Docker Claude reference lab, the reconnect command is the
native client login, with its **verified container name** substituted:

```bash
docker exec -it -u taxiway -e HOME=/home/taxiway -w /lab/work <reference-container> claude auth login --claudeai
```

`./taxiway auth <lab> claude-code` may only confirm that an existing credential
file is present; do not repeat it as a promise of renewing expired credentials.
For Lima, derive the native guest command from the actual reference driver and
lab ownership. For a confirmed expired host Codex subscription login, ask the
user to run the native `codex login`, then recheck the owned gateway path;
never execute host login or copy its credential files yourself.

### Pause, resume and finish

Say **pause** to stop new scheduling and preserve work. Coordinate in-flight
tests and their owned temporary cleanup; state what remains running. Say
**stop monitoring** to end the three loops without cancelling subjects. Say
**cancel <subject>** to stop its scheduling and coordinate its running work;
discarding commits, user edits or persistent labs needs explicit authorization.
Interrupting an interactive client does not guarantee remote jobs or child
tests stopped: reconcile them before restarting or cleaning.

Restart with the launcher and request **resume**, or use your client's native
session-resume mechanism and invoke the same skill. Reconstruct status from
worktrees, commits, issues, PRs, current-head checks and verified processes.
Use the prior conversation or an uncommitted local handoff for decisions not
represented there; never commit private run state, auth or logs. Treat missing
or unverifiable evidence as unknown. Resumption resets due times after this
reconciliation and must not launch duplicate agents or tests.

Before ending, explicitly state that monitoring stops. List each subject’s
remaining work and decisions, identify any still-running tests and their owners,
and provide the resume command or native session-resume action. Do not promise
automatic updates after the session ends. On resume, inspect live agents and
processes as well as recorded work and current-head checks before restarting
loops or assigning work; silence or a stale note does not prove a test stopped.

The final report links issues/PRs, states delivered behavior, actual validation
and remaining limits, and distinguishes code-ready, independently reviewed,
PR-open, merged and cleaned-up states. Do not claim a draft is prepared, a
diagnosis is running or a check passed without an actual artifact/process/result.
After an authorized merge, follow the
cleanup below; only report cleanup complete once verified.

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
