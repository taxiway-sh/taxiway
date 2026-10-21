---
name: taxiway-review
description: Use when a Taxiway change needs its independent review, when reviewing a commit range, diff or pull request, or when handling review findings and re-reviews.
---

# Taxiway independent review

Owns how the review required by the [invariants](../../../AGENTS.md#invariants)
is done. Read [`AGENTS.md`](../../../AGENTS.md) first.

## Reviewer

- A different, fresh agent, never the implementer and never an agent that
  wrote part of the change; [Running the reviewer](#running-the-reviewer) says
  how to start it.
- It reviews the exact commit range or diff (`git diff <base>...<head>`, with
  both SHAs), plus the issue and its acceptance criteria. It writes nothing:
  no edits, pushes or GitHub writes.

### Running the reviewer

- **Subagent by default.** Run the reviewer as a read-only subagent: it
  returns its report directly and leaves nothing behind. A subagent cannot
  start a subagent of its own, so an implementer that is a subagent hands back
  the exact range and leaves the review to the agent that delegated to it,
  unless it can run a child session and wait for its report itself.
- **Child session.** Only when the review needs its own environment, such as
  a long E2E run or a separate container, or when no subagent can be started.
  It clones from the remote, so the range must be pushed under the
  [approvals](../taxiway-workflow/SKILL.md#approvals), and nobody watches it,
  so it is created with `permission_mode: "auto"`, never `"default"` or
  `"plan"`, which wait on a prompt nobody sees; a caller that cannot grant
  `auto` does not use one. It is archived as soon as its report is read.
- **Stall check.** About 10 minutes after a child session's last event, and
  every 10 minutes after that until its report arrives, check its events for
  a pending prompt or a stopped turn: schedule the check with a tool the
  client provides, or, without one, keep the turn open and poll. If it is
  stuck, interrupt and archive it, then rerun the review as a subagent when
  one can be started.
- **Finish before handing back.** The agent that delegated the review does not
  hand back or end the session until the report is handled and its findings
  addressed; while a child session runs, it may end a turn after scheduling
  the stall check. When no reviewer can be started or the review cannot
  finish, say so plainly: the PR, when it is opened, lists the review as
  pending in Limitations and has no `Independent review` line in Validation.
  Never present your own review as independent.

## Checklist

Review adversarially: look for what is wrong, not for confirmation.

- **Acceptance.** The change meets the issue's acceptance criteria and the PR
  text matches what the diff does.
- **Behavior delta.** List existing behavior the diff changes or removes and
  probe at least one old-versus-new. An unannounced change in usage, output or
  defaults is a finding even if the fix is right.
- **Rule preservation.** Every rule removed, moved or merged still exists in
  its owner, or its removal is stated and justified.
- **Single owner.** No rule restated outside its owner; other files link to it.
- **Links.** Links and anchors resolve; `make lint` runs `check-agent-docs`.
  Pages under `docs/` and the landing page follow
  [`taxiway-docs`](../taxiway-docs/SKILL.md), [links](../taxiway-docs/SKILL.md#links)
  included.
- **Tests.** Claimed checks were actually run on this head, and skipped or
  blocked ones are reported as such. New behavior has tests where
  [Testing](../../../docs/contributing/testing.md) puts them, and lifecycle
  changes follow the [E2E scenario rule](../taxiway-testing/SKILL.md#e2e-scenarios).
  Each new assertion can fail and is mapped to the test that ran it (no early
  returns, no absence-only checks without a positive counterpart); E2E
  assertions follow [Writing E2E scenarios](../../../docs/contributing/testing.md#writing-e2e-scenarios).
- **Workarounds.** A workaround and any upstream claim follow
  [`taxiway-upstream`](../taxiway-upstream/SKILL.md#workaround-and-tracking-issue).
- **Leftovers.** After a removal or rename, no stale hint, help text or doc
  still points to the old name.
- **Commit hygiene.** [Conventional Commits](../../../docs/contributing/development.md#commit-messages)
  split by intention; no scratch files, local run state, branch names, run or review IDs,
  session narrative or unrelated changes;
  repository text in English.
- **Security.** The [security rules](../../../AGENTS.md#security) hold, as
  [`taxiway-security`](../taxiway-security/SKILL.md) applies them.
- **Mentions.** No project or organization unrelated to Taxiway and its
  documented dependencies.

## Findings

- Report either "no findings" or the findings by severity (blocking,
  should-fix, nit), each with its `file:line` and a concrete fix; then
  out-of-scope follow-ups and the validations the reviewer could not run.
- The maintainer's default is to fix every finding, nits included. A finding
  the implementer disagrees with goes to the maintainer with both positions.
- Re-review after fixes, and after any scope change, on the new range.
- Problems that are global or outside the change become issue proposals
  through [`taxiway-intake`](../taxiway-intake/SKILL.md), not fixes in this
  change.

## Review reports

The agent that ran the review publishes the report of each round as a
comment on the PR; the reviewer itself stays read-only, as
[Reviewer](#reviewer) says. Each comment gives:

- the round number;
- the range reviewed, as `<base>...<head>` with both full SHAs;
- the verdict: no findings, or findings to fix;
- the findings by severity, as [Findings](#findings) lists them;
- for each finding, the commit that fixes it, added by editing the comment
  once that commit is pushed, or why it is not fixed.

Rounds run before the PR is opened are posted as soon as it is. An
implementer that handed its range back leaves the comments to the agent that
ran the review. The comments are public and follow
[public writing](../taxiway-security/SKILL.md#public-writing).

The PR's Validation section states the review rounds as the line
`Independent review: N rounds`, followed by links to those comments, and that
findings were addressed. The agent that ran the review adds each link once its
comment is posted.

On an external contributor's PR, neither applies: the report goes into the
review drafted as
[`taxiway-contributions`](../taxiway-contributions/SKILL.md#external-pr) says,
posted only after the maintainer approves it.
