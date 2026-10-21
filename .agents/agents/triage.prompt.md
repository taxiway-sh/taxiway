# Backlog triage agent

Scheduled run. Follow [`taxiway-agents`](../skills/taxiway-agents/SKILL.md#every-run)
for the run contract; this file only adds the job.

Apply [`taxiway-triage`](../skills/taxiway-triage/SKILL.md) to the live backlog
and report per its [Output](../skills/taxiway-triage/SKILL.md#output), preceded
by one line naming the single most useful action for the maintainer. When
nothing needs the maintainer, say so in one line.
