---
name: taxiway-agents
description: Use when declaring, installing, checking or repairing Taxiway's scheduled agents, or when a scheduled agent run starts.
---

# Taxiway scheduled agents

Scheduled agents run Taxiway maintenance jobs between interactive sessions.
Read [`AGENTS.md`](../../../AGENTS.md) first. Installing, changing or removing
a scheduled agent follows the [approvals](../taxiway-workflow/SKILL.md#approvals).

## Declarations

Each agent is declared in this repository and changes only through reviewed PRs:

- `.agents/agents/<id>.json`: `id` (the file name), `title`, `schedule` (five
  cron fields, UTC), `model` (a model ID, or `default` for the account's
  default), `repositories` (`read` or `write` per repository),
  `workdir`, `skills`, `outputs` and `autonomy` (`ask` only, until
  per-action levels are defined in #196);
- `.agents/agents/<id>.prompt.md`: the job, read at the start of every run.

`read` means the agent never pushes to that repository; issue and PR writes
follow `autonomy`. This is declared, not enforced: a cloud session can usually
push, so agents cannot change their own instructions only because `main` is
protected and changes go through reviewed PRs. `make lint` checks the
declarations and the heartbeat schema
([`status.schema.json`](../../agents/status.schema.json)).

## State repository

`taxiway-sh/agents` is public and holds run state and improvement records,
never rules:

- `status/<id>.json`, the heartbeat written at the end of every run:
  `id`, `date` (UTC, ISO 8601), `result` (`ok`, `partial` or `failed`),
  `summary` (one sentence), `waiting` (items that need the maintainer, each with
  a link and the action) and `promptVersion` (the `taxiway` `main` SHA the run
  read);
- `deployed.json`: for each installed agent, its schedule, `model` and the
  `promptVersion` it was installed from. Session and scheduled task IDs stay
  out of it: install and check look them up, the session by tag and the
  scheduled task by name;
- `improvement/`, trials and rejected proposals, defined by
  [`taxiway-improve`](../taxiway-improve/SKILL.md#state-layout), and
  `learn/campaigns/`, history campaign progress, defined by
  [`taxiway-learn`](../taxiway-learn/SKILL.md#history-campaigns).

Everything written there is [public](../taxiway-security/SKILL.md#public-writing).

## Every run

Each agent runs in one standing session that keeps its context and its
attached repositories. A scheduled task bound to a standing session sends no
notification by itself: an agent that must alert the maintainer, like the
[watchdog](../../agents/watchdog.prompt.md), uses the session's
`PushNotification` tool, and only when something needs them.

The scheduled task always sends the same bootstrap prompt:

> Update the `taxiway` and `agents` clones on `main` with
> `git fetch origin main` and `git merge --ff-only origin/main`, then read
> `taxiway/.agents/agents/<id>.prompt.md` and follow it. If a clone cannot
> fast-forward, report it and do not reset it.

A clone whose fetch or fast-forward fails, or that is not on `main`, is
reported: the run writes a `failed` heartbeat if the `agents` clone allows it,
then stops. A run never resets or force-updates a clone.

Each run then:

1. records the `taxiway` `main` SHA as `promptVersion`;
2. does the job in its prompt file at the declared `autonomy`
   ([approvals](../taxiway-workflow/SKILL.md#approvals));
3. writes `status/<id>.json`, commits and pushes it to `taxiway-sh/agents`;
4. ends with its report in the agent's session, where the maintainer reads and
   answers it; the heartbeat's `waiting` lists what needs them. Unattended
   reports are in English; when someone replies in the session, continue in
   their language.

A run that cannot finish still writes its heartbeat with `failed` or `partial`
and says why.

## Install

For each declaration, or the one `id` requested:

1. Look up the agent's session with `list_sessions` (tag
   `taxiway-agent:<id>`; if the tag filter is refused, list your own sessions
   and match the title `Taxiway agent: <id>`), and its scheduled task by the
   same name, paging through `list_triggers`. Reuse them when they exist;
   never create a second one for the same agent.
2. Otherwise create one standing session for the agent from the `taxiway`
   repository, with the declared `model`, titled `Taxiway agent: <id>` and
   tagged `taxiway-agent:<id>`, and send a setup-only first prompt: attach
   `taxiway-sh/agents` with `add_repo` (`access: push`), verify both clones
   and the tools, then stop. Tools cannot be pre-approved from another
   session, so the maintainer approves that one `add_repo` call in the
   agent's session; the repository then stays attached as a source of the
   session.
3. Create a scheduled task named `Taxiway agent: <id>`, bound to that session,
   with the declared schedule and the bootstrap prompt. Never use a new
   session per run: such a session has no repository access.
4. When the schedule or the prompt changed, update the existing scheduled task.
   When the model changed, archive the session, create a new one as in
   step 2 and rebind the task.
5. Record the schedule, `model` and `promptVersion` in `deployed.json`.

Installation is idempotent: running it again changes nothing that already
matches the declarations.

## Check

Find each agent's session and scheduled task as in [Install](#install) step 1,
then report:

- the last heartbeat's date and result against the
  schedule (late, failed or missing is a finding);
- the scheduled task: present, enabled, last run;
- the session: present and not failed;
- a `promptVersion` behind `main`, which shows the run read older
  instructions.

A check writes nothing. Repairs go through [Install](#install).
