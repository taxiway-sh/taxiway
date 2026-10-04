---
name: taxiway-workflow
description: Use when starting or resuming a Taxiway contribution session, choosing repository issues, or coordinating implementation, live tests and independent review.
---

# Taxiway contribution workflow

Read `AGENTS.md`, then the [contribution workflow](../../../docs/contributing/development.md#contribution-workflow)
from the repository root. Those sources own policy and procedures; this skill
coordinates them in either client. Paths in commands are relative to the checkout.

## Intake and reconciliation

Ask first: **new need/problem, or suggestions from the backlog?** A request to
resume a named subject already answers this choice; reconcile it directly.
Read-only inspection needs no new approval. Inspect worktrees, branches, issues,
PRs, check results and available local handoffs. Check ownership and whether
tests/agents are still running before assigning or rerunning work. Unknown past
results remain unknown, and old-head results do not validate a new commit.
Do not claim drafts, diagnoses, tests or state writes exist without evidence.

For new needs, analyze evidence, check duplicates, propose issue scope and
acceptance criteria using `docs/contributing/issues.md`; create when authorized.
For backlog, propose a short list using explicit priority first, project value,
dependencies, readiness and capacity. Identify missing priorities and existing
assignments. Let the user select work; do not silently relabel or start it.

## Execution

Use a dedicated worktree/implementer per subject and a different reviewer.
Resolve review findings and verify corrections. If delegation is unavailable,
report the missing review rather than review your own work as independent.
Follow the documented tests, commits, PR and authorized merge/cleanup procedure.
Preserve unrelated work and credentials. Necessary acceptance fixes stay in
scope; unrelated discoveries become sanitized, deduplicated issue proposals.
Ask whether to create them unless already authorized.

## Active-session loops

The coordinator owns these independent checks; use client tools already
available, not a provider-specific workflow runtime. Default intervals: overall
status 300 seconds, backlog 900 seconds; defaults: two agents including the
coordinator, one heavy Docker/Lima run. Explicit startup settings override them
within actual capacity. Record next due times, inspect at bounded task/wait
boundaries, and keep waits shorter than the status interval. Avoid blocking
long-running tools; use asynchronous execution when available. Report timer or
delegation limitations immediately. No loops survive session termination.

- **Status:** give an initial report and next expected update; at each due time
  report every subject's stage, evidence, changes, waits, pending user action
  and next step, even if unchanged. Report milestones/blockers promptly and
  coalesce overlapping reports. Never invent percentages or finish dates.
- **Authentication:** on a live-test auth failure, inspect only sanitized
  native errors and existing helper results. Distinguish missing/rejected auth
  from gateway timeout/network/model errors. For confirmed auth failure, give
  one exact native reconnection command for the owned reference context, ask
  the user to reconnect, verify a bounded real request, then propagate only to
  authorized test labs with existing helpers. Until #116 provides preflight,
  do not claim file-presence checks prove validity or implement token refresh.
  Keep unrelated work moving; one pending request per reference blocker.
- **Triage:** at startup/backlog request and each due time, refresh proposals
  against assignments, priority and capacity. Do not repeat unchanged
  recommendations unnecessarily or start unselected work.

Pause stops scheduling and preserves work; stop monitoring ends loops. Cancel
stops that subject's scheduling, coordinates in-flight tests and owned cleanup,
and requires authorization to discard persistent work. On resume, reconcile
state before restarting loops. End with links, actual checks, limitations and
distinct implementation/review/PR/merged/cleanup states.
