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
  cron fields, UTC), `repositories` (`read` or `write` per repository),
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

`taxiway-sh/agents` is public and holds run state only, never rules:

- `status/<id>.json`, the heartbeat written at the end of every run:
  `id`, `date` (UTC, ISO 8601), `result` (`ok`, `partial` or `failed`),
  `summary` (one sentence), `waiting` (items that need the maintainer, each with
  a link and the action) and `promptVersion` (the `taxiway` `main` SHA the run
  read);
- `deployed.json`: for each installed agent, its session ID, scheduled task ID,
  schedule and the `promptVersion` it was installed from.

Everything written there is public: no secrets, host details, private paths or
raw logs.

## Every run

The scheduled task always sends the same bootstrap prompt:

> Reset the `taxiway` and `agents` clones to `origin/main`, then read
> `taxiway/.agents/agents/<id>.prompt.md` and follow it.

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

1. Read `deployed.json`. Reuse a recorded session and scheduled task when they
   still exist; never create a second one for the same agent.
2. Otherwise create one standing session for the agent from the `taxiway`
   repository, attach `taxiway-sh/agents` with `add_repo` (`access: push`), and
   send a setup-only first prompt: verify both clones and the tools, then stop.
3. Create a scheduled task bound to that session with the declared schedule and
   the bootstrap prompt. Never use a new session per run.
4. When the schedule or the prompt changed, update the existing scheduled task.
5. Record the IDs, schedule and `promptVersion` in `deployed.json`.

Installation is idempotent: running it again changes nothing that already
matches the declarations.

## Check

For each declared agent, report:

- the last heartbeat's date and result against the schedule (late, failed or
  missing is a finding);
- the scheduled task: present, enabled, last run;
- the session: present and not failed;
- a `promptVersion` behind `main`, which shows the run read older instructions.

A check writes nothing. Repairs go through [Install](#install).
