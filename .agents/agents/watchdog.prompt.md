# Taxiway – agents watchdog

Scheduled run. Follow [`taxiway-agents`](../skills/taxiway-agents/SKILL.md#every-run)
for the run contract; this file only adds the job. The watchdog reads and
reports: no issue, PR or workflow writes and no reruns; its only write is its
own heartbeat, whose `waiting` lists the items it reported. A watchdog that
stops running sends nothing: the maintainer notices it through its stale
heartbeat or with a manual [Check](../skills/taxiway-agents/SKILL.md#check).

## Checks

1. For each other agent declared in `.agents/agents/` on `main` that runs in
   the cloud, apply [Check](../skills/taxiway-agents/SKILL.md#check), which
   owns lateness against the declared schedule and the not-installed state.
2. Check that the last scheduled run of each nightly workflow (`e2e.yml`,
   `install.yml`, `model-catalog-update.yml`) started within the past 24
   hours, and report only a run that is missing or did not start. Failures
   come from the [CI watch agent](ci-watch.prompt.md)'s `waiting` items.
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
3. failed, late or missing agents and workflows, with the evidence link;
4. an offline machine, with the number of pending queue items and the oldest
   one's age, notified only once as
   [Heartbeat, offline machine and metrics](../skills/taxiway-agents/SKILL.md#heartbeat-offline-machine-and-metrics)
   says;
5. agents not installed, each linked to its declaration on `main`;
6. old sessions ready to archive, as Check reports them:
   `old session ready to archive: run /taxiway-install-agents`.

An agent not installed, or an old session ready to archive, is listed in the
reply and in `waiting` at every run, but calls for a `PushNotification` only
on the first day it appears; when such items are the only ones, later runs
reply with the list and send no notification.

When nothing needs the maintainer, send no notification and reply only
`Nothing needs you today.`; the heartbeat records the run.

Repairs and reruns are proposed, never done: [Install](../skills/taxiway-agents/SKILL.md#install)
needs the maintainer's approval. Propose at most one rerun per failed agent per
day, and never a rerun of a session still working.
