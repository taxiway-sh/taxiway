---
name: taxiway-workflow
description: Use when starting, resuming or finishing a Taxiway contribution session, routing a need to its owner, or coordinating agents, capacity, approvals and status.
---

# Taxiway contribution workflow

This skill is the session entry point and the single owner of the contribution
workflow procedure. Read [`AGENTS.md`](../../../AGENTS.md) first: it holds the
invariants and routes each need to the skill or document that owns it. Paths in
commands are relative to the checkout; humans start sessions as described in
[Start a session](../../../docs/contributing/development.md#start-a-session).

## Intake and reconciliation

Ask first: **new need/problem, or suggestions from the backlog?** Selecting or
resuming a named subject answers this question; reconcile it directly. It does
not authorize publishing, merging or starting additional subjects.

Reconcile before assigning anything. Inspect `git worktree list`, branches,
issues, PRs, check results and available local handoffs, using `gh` with
bounded fields and results (see the [invariants](../../../AGENTS.md#invariants)). Check
ownership and whether agents or tests are still running before assigning or
rerunning work. Historical notes do not prove a process is running or a check
passed; old-head results do not validate a new commit; unknown past results
remain unknown. Do not claim drafts, diagnoses, tests or state writes exist
without an actual artifact, process or result.

- **New need:** follow [`taxiway-intake`](../taxiway-intake/SKILL.md).
- **Backlog:** follow [`taxiway-triage`](../taxiway-triage/SKILL.md); the user
  selects the work.

Several sessions, cloud or local, may work on Taxiway at once, scheduled agents
included, and none sees another's worktrees or processes: the state of work is
on GitHub (issues, branches, PRs, checks), and that of scheduled agents in the
[state repository](../taxiway-agents/SKILL.md#state-repository).
[Selection](../taxiway-issue/SKILL.md#1-selection) says how a session checks
and shows that a subject is taken. The coordinator session is the maintainer's
entry point for status, decisions and priorities: it delegates work to
subagents or dedicated sessions and reads that state rather than relying on
memory; decisions not recorded there come from the prior conversation or a
local handoff, as [Resume](#pause-resume-and-finish) says.

## Approvals

| Needs explicit user approval | Needs no new approval |
|---|---|
| Creating an issue, unless already authorized | Read-only inspection and reconciliation |
| Starting a subject the user has not selected | Implementation, tests and review within a selected subject |
| Pushing a branch or publishing a PR | Proposing issues, PR text or upstream reports without publishing them |
| Merging a PR | [Post-merge cleanup](../taxiway-issue/SKILL.md#post-merge-cleanup) after an authorized merge |
| Force-pushing | Cleaning resources owned by a scenario with existing helpers |
| Applying, removing or creating labels; comments, assignments, closures, reopenings and reviews on issues and PRs, unless already authorized | Recording in the issue a decision the maintainer has just given |
| Releases: follow [`taxiway-release`](../taxiway-release/SKILL.md) | |
| Upstream issues, PRs, comments, pushes or forks ([`taxiway-upstream`](../taxiway-upstream/SKILL.md#approvals)) | |
| Real-provider (paid) live checks | |
| Installing, changing or removing a [scheduled agent](../taxiway-agents/SKILL.md) | |
| Discarding commits, user edits or persistent labs | |

A scheduled agent takes the actions covered by an
[action class](../taxiway-agents/SKILL.md#action-classes) at the level its
declaration sets; every other action here keeps this matrix, and the
[never list](../taxiway-agents/SKILL.md#never) applies whatever the level.

Upstream publication follows
[`taxiway-upstream`](../taxiway-upstream/SKILL.md#approvals). A pending
approval blocks only that action on that subject: keep other authorized,
independent implementation, tests and reviews moving within capacity. An
approval wait alone is not a reason to end the session. If every remaining
action is blocked, report the pending decisions and whether the client can stay
active; do not invent a background runner or promise future turns.

Before a new or changed agent or skill writes to GitHub for the first time,
exercise it with a dry run that writes nothing and show the result to the
maintainer.

## Execution

- **Routing.** A selected issue follows
  [`taxiway-issue`](../taxiway-issue/SKILL.md) with one implementation agent;
  its review follows [`taxiway-review`](../taxiway-review/SKILL.md) and its PR
  follow-up [`steward`](../steward/SKILL.md). A change to pages under `docs/`
  or to the landing site also follows [`taxiway-docs`](../taxiway-docs/SKILL.md).
- **Capacity.** There is no fixed agent quota: size concurrency to measured
  host capacity (memory, swap, running VMs and containers) and client limits.
  The launcher's `--max-agents` is an optional ceiling; `--max-heavy-tests`
  limits concurrent heavy Docker and Lima runs. Identify context
  ownership, defer work when memory, network pools or client capacity are
  exhausted, and do not duplicate agents or tests. Diagnose and retry
  narrowly; never globally prune resources or delete another context. Prefer
  asynchronous execution for long-running tools.
- **Preflight.** Before tests or live runs, confirm the working directory,
  branch and `HEAD`, and that the main checkout has no stray edits. Rebuild
  `./taxiway` from the worktree ([build](../../../docs/contributing/development.md))
  and invoke it by that path. Use a temporary lab after driver, mount or
  template changes. Check host memory, swap and
  idle VMs before heavy runs. Verify every command of a plan written by
  another session against the real CLI (`--help`) before running or handing
  it over.
- **Run results.** A run cut short by a usage limit, crash or kill has no
  verdict: report it and do not relaunch it until the user says to resume. A
  create that passes only on retry, or a timeout on a saturated host, is
  inconclusive; report results as
  [live validation](../../../docs/contributing/live-tests.md) requires.
- **Scope.** Acceptance-critical fixes stay in the subject. Unrelated
  discoveries and suspected dependency bugs go through
  [`taxiway-intake`](../taxiway-intake/SKILL.md). Preserve unrelated work and
  credentials.
- **Studies and plans** stay out of Git
  ([invariants](../../../AGENTS.md#invariants)): the maintainer first reviews
  them in a private page or a file outside the repository; once approved, they
  are posted in English as an issue comment.

## Interaction

- A question or a request for an opinion is not a request to act: answer it
  and propose the action; act only when asked, within the
  [approvals](#approvals).
- Commands handed to the user are single lines that run as pasted from the
  worktree root, with worktree-relative paths, lab names per
  [live validation](../../../tests/live/AGENTS.md) and the expected output of
  each step; explain non-default flags.
- Ask questions through the client's interactive question tool when one is
  available. Re-ask a question that went unanswered; silence is not an answer.
- Converse in the user's language. Repository artifacts (issues, PRs, commits,
  code and docs) stay in English.
- When a term looks distorted by voice dictation (an unknown name, an odd
  command or label), confirm it before acting on it.

## Decisions

Record a maintainer decision in the rule it creates, in the skill or document
that owns that rule (see the routing table in `AGENTS.md`), with a link to the
issue or PR where it was taken. Do not write ADRs or separate decision logs, and
do not restate the rule elsewhere: link to its owner. Refused improvement
proposals and learning candidates are the exception: they are recorded as
[rejected proposals](../taxiway-improve/SKILL.md#rejected-proposals).

When the user corrects you, or answers a question that applies beyond the
current task, propose on the spot to record it this way in its owner. When you
do something that no skill or document describes, such as a new procedure, a
workaround or a check that turned out to matter, say so briefly and offer the
same.

## Status on demand

When the user asks, and promptly at milestones or blockers, report each
subject's stage, evidence, changes, waits, pending user action and next step,
including unchanged-but-running work. Coalesce overlapping reports, keep one
actionable request per blocker, and never invent percentages or finish dates.

## Pause, resume and finish

- **Pause** stops new scheduling and preserves work. Coordinate in-flight tests
  and their owned temporary cleanup; state what remains running.
- **Cancel <subject>** stops its scheduling and coordinates its running work;
  discarding commits, user edits or persistent labs needs explicit approval.
  Interrupting a client does not guarantee remote jobs or child tests stopped;
  reconcile them before restarting or cleaning.
- **Resume:** read this skill again and reconcile worktrees, commits, issues,
  PRs, current-head results, live agents and processes before assigning work.
  Use the prior conversation or an uncommitted local handoff for decisions not
  represented there; never commit private run state, auth or logs. A stale
  handoff or absent message is not proof that a test stopped; preserve
  uncertain work until ownership and state are established, and do not launch
  duplicate agents or tests.
- **Finish:** state that nothing continues after the session ends. Close
  subagents and child sessions whose mission has ended and list any still
  running. List each subject's remaining work and pending decisions, the
  branches and PRs it leaves open, any still-running tests with their owners,
  and the resume action. The final report links issues and PRs, states
  delivered behavior, actual checks and remaining limits, and distinguishes
  code-ready, independently reviewed, PR-open, merged and cleaned-up states.
- **Frictions observed and rules to record:** end by running
  [`taxiway-learn`](../taxiway-learn/SKILL.md) on the session and presenting
  its candidates the way it says. Its list includes the frictions observed,
  each once: missing, ambiguous or contradictory rules, workarounds used,
  checks that could not run and refused tools. Nothing is written until the
  maintainer picks.
