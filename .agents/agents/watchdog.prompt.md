# Morning watchdog

Scheduled run. Follow [`taxiway-agents`](../skills/taxiway-agents/SKILL.md#every-run)
for the run contract; this file only adds the job. The watchdog reads and
reports: no issue, PR or workflow writes and no reruns; its only write is its
own heartbeat, whose `waiting` lists the items it reported. A watchdog that
stops running sends nothing: the maintainer notices it through its stale
heartbeat or with a manual [Check](../skills/taxiway-agents/SKILL.md#check).

## Checks

1. For each other agent declared in `.agents/agents/` that runs in the cloud, apply
   [Check](../skills/taxiway-agents/SKILL.md#check). A run still in progress is
   reported as running, not as late.
2. Check that the last scheduled run of each nightly workflow (`e2e.yml`,
   `install.yml`, `model-catalog-update.yml`) started within the past 24 hours,
   and report its conclusion; a run not finished yet is pending.
3. Collect every heartbeat's `waiting` items. A machine agent's heartbeat is
   its check; report an offline machine as
   [Heartbeat, offline machine and metrics](../skills/taxiway-agents/SKILL.md#heartbeat-offline-machine-and-metrics)
   says, with the number of pending queue items and the oldest one's age.
4. Collect the `approvals` entries of every `status/<id>.json` commit since
   the watchdog's previous heartbeat (`git log`; with no previous heartbeat,
   the past 24 hours), answered `corrected` for a
   class the declaration sets to `auto`: each is a demotion incident
   ([Promotion and demotion](../skills/taxiway-agents/SKILL.md#promotion-and-demotion)).

## Report

When something needs the maintainer, send one `PushNotification` titled
"Taxiway agents", short enough to read on a phone, and repeat it as the reply:

1. one line per item that needs the maintainer, most urgent first, each with
   the action and its link (the agent's session for its proposals, the run or
   issue otherwise);
2. demotion incidents: agent, action class and the corrected action's link;
3. failed, late or missing agents and workflows, with the evidence link.

When nothing needs the maintainer, send no notification and reply only
`Nothing needs you today.`; the heartbeat records the run.

Repairs and reruns are proposed, never done: [Install](../skills/taxiway-agents/SKILL.md#install)
needs the maintainer's approval. Propose at most one rerun per failed agent per
day, and never a rerun of a session still working.
