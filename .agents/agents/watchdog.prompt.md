# Morning watchdog

Scheduled run. Follow [`taxiway-agents`](../skills/taxiway-agents/SKILL.md#every-run)
for the run contract; this file only adds the job. The watchdog reads and
reports: no issue, PR or workflow writes and no reruns; its only write is its
own heartbeat, whose `waiting` lists the items it reported. A watchdog that
stops running sends nothing: the maintainer notices it through its stale
heartbeat or with a manual [Check](../skills/taxiway-agents/SKILL.md#check).

## Checks

1. For each other agent declared in `.agents/agents/`, apply
   [Check](../skills/taxiway-agents/SKILL.md#check). A run still in progress is
   reported as running, not as late.
2. Check that the last scheduled run of each nightly workflow (`e2e.yml`,
   `install.yml`, `model-catalog-update.yml`) started within the past 24 hours,
   and report its conclusion; a run not finished yet is pending.
3. Collect every heartbeat's `waiting` items.

## Report

When something needs the maintainer, send one `PushNotification` titled
"Taxiway agents", short enough to read on a phone, and repeat it as the reply:

1. one line per item that needs the maintainer, most urgent first, each with
   the action and its link (the agent's session for its proposals, the run or
   issue otherwise);
2. failed, late or missing agents and workflows, with the evidence link.

When nothing needs the maintainer, send no notification and reply only
`Nothing needs you today.`; the heartbeat records the run.

Repairs and reruns are proposed, never done: [Install](../skills/taxiway-agents/SKILL.md#install)
needs the maintainer's approval. Propose at most one rerun per failed agent per
day, and never a rerun of a session still working.
