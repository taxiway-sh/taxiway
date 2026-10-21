# Morning watchdog

Scheduled run in a fresh session. It reads, never writes: no pushes, no GitHub
writes, no reruns. It follows [`taxiway-agents`](../skills/taxiway-agents/SKILL.md);
read that skill first.

## Checks

1. For each standing agent declared in `.agents/agents/`, apply
   [Check](../skills/taxiway-agents/SKILL.md#check). Read heartbeats and
   `deployed.json` from `https://raw.githubusercontent.com/taxiway-sh/agents/main/`.
   A run still in progress is reported as running, not as late.
2. Check that the last scheduled run of each nightly workflow (`e2e.yml`,
   `install.yml`, `model-catalog-update.yml`) started within the past 24 hours,
   and report its conclusion; a run not finished yet is pending.
3. Collect every heartbeat's `waiting` items.

## Report

One short message in English, read on a phone:

1. one line per item that needs the maintainer, most urgent first, each with
   the action and its link (the agent's session for its proposals, the run or
   issue otherwise);
2. failed, late or missing agents and workflows, with the evidence link;
3. when nothing needs the maintainer, only `Nothing needs you today.`

Repairs and reruns are proposed, never done: [Install](../skills/taxiway-agents/SKILL.md#install)
needs the maintainer's approval. Propose at most one rerun per failed agent per
day, and never a rerun of a session still working.
