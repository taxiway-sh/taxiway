---
name: taxiway-issue
description: Use when working on a selected Taxiway issue, from selection through worktree, branch, commits, checks, PR publication, merge and post-merge cleanup, or when stacking work on an unmerged branch.
---

# Taxiway issue lifecycle

Owns the path of one selected issue from selection to cleanup. Read
[`AGENTS.md`](../../../AGENTS.md) first; every GitHub write and every push
follows the [approvals matrix](../taxiway-workflow/SKILL.md#approvals).

## 1. Selection

- Work only on an issue the maintainer selected
  ([workflow](../taxiway-workflow/SKILL.md#intake-and-reconciliation)).
- Check ownership and existing work before starting: assignees, open PRs that
  reference the issue, `git worktree list`, local and remote branches. Resume
  or coordinate existing work instead of duplicating it.
- Remove `needs-triage` as the
  [label conventions](../../../docs/contributing/issues.md#label-conventions)
  say.

## 2. Worktree and branch

One worktree per issue, named `.worktrees/issue-<n>-<subject>`, on a branch
`<type>/<n>-<subject>` whose type is a
[commit type](../../../docs/contributing/development.md#commit-messages), for
example `fix/93-gastown-deacon-startup`. Run from the main checkout, never from
another worktree, so worktrees are not nested:

```bash
git fetch origin
git worktree add -b <type>/<n>-<subject> .worktrees/issue-<n>-<subject> origin/main
```

- Start from current `main`. When the work depends on an unmerged branch, start
  from that branch instead (`origin/<base-branch>` in the command above) and
  open the PR as the [PR template](../../../.github/pull_request_template.md)
  says for a stacked PR.
- `.worktrees/` is listed in `.gitignore`, so worktrees stay out of
  `git status`.
- Each worktree has its own runtime context through its `.envrc`
  ([Source checkout runtime](../../../docs/contributing/development.md#source-checkout-runtime)).

## 3. Implementation

Commit by intention with [Conventional Commits](../../../docs/contributing/development.md#commit-messages),
within the [scope rule](../taxiway-workflow/SKILL.md#execution). Each commit
body references the issue with `Refs #<issue>`. While the branch is unpushed,
fold review fixes into the commit they belong to (`git commit --fixup=<sha>`,
then `GIT_SEQUENCE_EDITOR=: git rebase -i --autosquash <base>`, which needs no
editor); once it is pushed, fixes are new commits,
since rewriting it needs a force-push ([Conflicts](../steward/SKILL.md#conflicts)).

## 4. Checks

- Choose the checks with [`taxiway-testing`](../taxiway-testing/SKILL.md) and
  run them with [Testing](../../../docs/contributing/testing.md); durable E2E
  assertions and the merge criterion follow the
  [E2E scenario rule](../taxiway-testing/SKILL.md#e2e-scenarios).
- To test the feature like a manual tester, pick Docker or Lima as
  [`taxiway-testing`](../taxiway-testing/SKILL.md#live-validation-docker-or-lima)
  says and follow [Live validation](../../../docs/contributing/live-tests.md)
  (including its [Lima recipes](../../../docs/contributing/live-tests.md#lima-driver-and-recording-recipes))
  and [`tests/live/AGENTS.md`](../../../tests/live/AGENTS.md). When a live check
  reports an authentication failure, follow
  [When authentication fails](../../../docs/contributing/live-tests.md#when-authentication-fails)
  and keep unrelated work moving.
- Record each check with its head commit and result; it goes into the PR's
  Validation section.

## 5. Independent review

Follow [`taxiway-review`](../taxiway-review/SKILL.md) on the exact commit range,
fix the findings, and re-review as it says.

## 6. Pull request

Push the branch and open the PR under the approvals matrix. Its title is a
[Conventional Commits](../../../docs/contributing/development.md#commit-messages)
subject; its body follows the
[PR template](../../../.github/pull_request_template.md); its labels follow
[Pull request labels](../../../docs/contributing/issues.md#pull-request-labels).
The PR that completes an issue links it with a closing keyword, so the merge
closes it, as the PR template and that section say.

## 7. CI and follow-up

Follow [`steward`](../steward/SKILL.md) until the PR is merged or closed.

## 8. Merge

Merge only under the [invariants](../../../AGENTS.md#invariants) and the
[E2E scenario rule](../taxiway-testing/SKILL.md#e2e-scenarios).

## Post-merge cleanup

An authorized merge also authorizes cleaning up that PR's development instance,
worktree and feature branch; no separate confirmation is needed.

1. Coordinate with the worktree's agent and wait for its tests to finish.
   Check for uncommitted work and preserve anything not included in the PR.
2. Load that worktree's `.envrc` and identify its context id and lab state. Run
   `./taxiway list`, then `./taxiway destroy --yes` from that context to remove
   all its labs, gateways, observability, proxy and generated state. Do this
   before removing the worktree or its runtime assets.
3. Verify that the context's Docker containers, networks, volumes and Lima labs
   are gone. Investigate leftovers by their exact context ownership; never use a
   global prune or remove another worktree's resources. If destroy or
   verification fails, keep the worktree for recovery and report the failure.
4. After confirming the PR was merged, remove the clean worktree and its local
   feature branch. List the open PRs whose base is that branch, delete the
   remote feature branch, then confirm each one was retargeted to `main` and is
   still open, and follow
   [Stacked PRs](../steward/SKILL.md#stacked-prs). Do not discard uncommitted
   work or unrelated commits, and do not remove `main`.
5. Fast-forward local `main`, then update the other active branches at a safe
   point with their agents so ongoing edits and tests are preserved, as in
   [Conflicts](../steward/SKILL.md#conflicts), and rerun affected checks.
6. For each issue the PR, or a stacked PR merged into its branch, links with
   `Refs` or `Part of`, and each issue they link with `Fixes` or `Closes` that
   is still open, reread its acceptance criteria. When its criteria are met,
   propose closing it as completed, with a comment that names the PRs that
   delivered it; otherwise propose ticking the criteria now met, or a comment
   on an external contributor's issue. These wait for approval under the
   [approvals](../taxiway-workflow/SKILL.md#approvals); cleanup is complete
   with them pending. A stacked PR merged into a branch other than `main`
   closes nothing: its issues are rechecked in the cleanup of the PR that
   brings the change to `main`. Criteria that need a later step, such as an
   installation or a week of runs, keep the issue open until that step is done.

Report cleanup complete only once verified.
