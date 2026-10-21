# Taxiway – model catalog agent (catalog PR)

Scheduled run. Follow [`taxiway-agents`](../skills/taxiway-agents/SKILL.md#every-run)
for the run contract; this file only adds the job: bring the model catalog PR
to green, or report why not.

1. **Find.** Read the last scheduled run of `model-catalog-update.yml`
   (**Prepare model catalog update**) and the open draft PR from
   `automation/model-catalog`. With no open PR, report the run's date and
   conclusion in one line; a failed run without a PR is a finding.
2. **Review** the PR with the
   [model catalog review](../skills/taxiway-upstream/references/model-catalog.md):
   retired defaults, failing checks and their root cause, compatibility with
   the pinned gateway.
3. **Repair.** When a fix belongs in the PR (a catalog entry, a default that
   names a retired model, a test that asserts it), prepare it in a worktree
   created for this run, so the `taxiway` clone stays on `main`:
   `git fetch origin automation/model-catalog`, then
   `git worktree add --detach <path> origin/automation/model-catalog`. Run the
   checks the fix needs (`taxiway-testing`, or
   [Testing](../../docs/contributing/testing.md) while that skill is not on
   `main`), then propose the push (`push-pr-branch`) with the diff and the
   results. After the OK, push with
   `git push origin HEAD:automation/model-catalog`, never with force; when
   the push is refused because the branch moved, fetch and redo the fix.
   Remove the worktree after the push or the refusal.
4. **Ready and merge.** Ask for the OK to mark the PR ready (`mark-pr-ready`)
   only when every recorded check passes on its head, and to merge it
   (`merge-pr`) only once it is ready.
5. **Deadline.** Every scheduled run of `model-catalog-update.yml` (23:37 UTC)
   rewrites `automation/model-catalog` and drafts the PR again. A pushed
   repair, the ready state and the merge must all happen before the next
   such run, or the repair is lost: say so with the deadline in each
   proposal. A fix that must outlive that run belongs in the updater or on
   `main`: draft a tracking issue through
   [`taxiway-intake`](../skills/taxiway-intake/SKILL.md) (`create-issue`)
   instead.

Report per the review's
[Output](../skills/taxiway-upstream/references/model-catalog.md#output),
preceded by one line naming the single most useful action for the maintainer.
When nothing changed since the previous run, say so in one line.
