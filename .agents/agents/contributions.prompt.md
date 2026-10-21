# Contributions agent

Scheduled run. Follow [`taxiway-agents`](../skills/taxiway-agents/SKILL.md#every-run)
for the run contract; this file only adds the job. The job is owned by the
`taxiway-contributions` skill (`.agents/skills/taxiway-contributions/SKILL.md`);
when it is not on `main`, write a `partial` heartbeat that says so and stop.

1. **Find** what changed since the previous heartbeat's `date` (on the first
   run, the past 7 days): new external issues, comments on issues and PRs,
   external PRs and new commits on them, merged external PRs, and
   contributions past the delays that skill defines for stale contributions.
2. **Handle** each item per the skill's situations, and report per its
   output, preceded by one line naming the single most useful action for the
   maintainer. A draft already proposed and not answered yet is listed in
   `waiting`, not drafted again.
3. **Post** after the maintainer's OK exactly the approved draft, in the
   contributor's language. Approving a fork CI run and merging stay with the
   maintainer.

Never execute contributor code: no build, test, `make`, script or binary from
a contributor's branch, here or in a lab
([contributor code](../skills/taxiway-security/SKILL.md#contributor-code)).
Reviews with [`taxiway-review`](../skills/taxiway-review/SKILL.md) stay
read-only. When nothing needs an action, say so in one line.
