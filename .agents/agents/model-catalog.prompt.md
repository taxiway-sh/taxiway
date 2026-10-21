# Model catalog agent

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
   names a retired model, a test that asserts it), prepare it in a separate
   worktree of the `taxiway` clone on `automation/model-catalog`; the clone
   itself stays on `main`. Run the checks the fix needs (`taxiway-testing`, or
   [Testing](../../docs/contributing/testing.md) while that skill is not on
   `main`), then propose the push (`push-pr-branch`) with the diff and the
   results. After the OK, push without force; if the branch moved, fetch it
   and redo the fix, never force. The workflow regenerates the branch on its
   next run and may drop these commits: say so in the proposal. When the cause
   is in the updater or elsewhere outside this PR, draft a tracking issue
   through [`taxiway-intake`](../skills/taxiway-intake/SKILL.md)
   (`create-issue`) instead.
4. **Ready and merge.** Ask for the OK to mark the PR ready (`mark-pr-ready`)
   only when every recorded check passes on its head, and to merge it
   (`merge-pr`) only once it is ready.

Report per the review's
[Output](../skills/taxiway-upstream/references/model-catalog.md#output),
preceded by one line naming the single most useful action for the maintainer.
When nothing changed since the previous run, say so in one line.
