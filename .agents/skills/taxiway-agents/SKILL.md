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

- `.agents/agents/<id>.json`: `id` (the file name), `title` (the name of its
  session and scheduled task, `Taxiway – <role> agent (<scope>)` with an en
  dash, for example `Taxiway – triage agent (backlog)`; the watchdog is
  `Taxiway – agents watchdog`), `schedule` (five cron fields, UTC), `model`
  (a model ID, or `default` for the account's default), `repositories`
  (`read` or `write` per repository), `workdir`, `skills`, `outputs` and
  `autonomy` (see [Autonomy](#autonomy));
- `.agents/agents/<id>.prompt.md`: the job, read at the start of every run.

`read` means the agent never pushes to that repository. `write` on
`taxiway-sh/taxiway` means pushing, without force, to branches other than
`main`; `make lint` requires it exactly when `push-pr-branch` or
`change-agent-system` is at `ask` or `auto`. Issue and PR writes follow
[Autonomy](#autonomy). This is declared, not enforced: a cloud session can
usually push, so agents cannot change their own instructions only because
`main` is protected and changes go through reviewed PRs. A Taxiway workflow may
regenerate its bot branch (`automation/*`) and drop an agent's commits there.
`make lint` checks the declarations and the heartbeat schema
([`status.schema.json`](../../agents/status.schema.json)).

## State repository

`taxiway-sh/agents` is public and holds run state and improvement records,
never rules:

- `status/<id>.json`, the heartbeat written at the end of every run:
  `id`, `date` (UTC, ISO 8601), `result` (`ok`, `partial` or `failed`),
  `summary` (one sentence), `waiting` (items that need the maintainer, each with
  a link and the action), `promptVersion` (the `taxiway` `main` SHA the run
  read) and, optionally, `approvals`: the maintainer's answers this run
  recorded, each with `class` (an [action class](#action-classes)),
  `proposal` (its id or link), `answer` (`ok`, `refused` or `corrected`),
  `date` (of the answer) and `link` (the GitHub item or the action taken).
  A heartbeat holds at most 50 answers; the rest are carried to the next run.
  The history of `status/<id>.json` is the approval log;
- `deployed.json`: for each installed agent, its schedule, `model` and the
  `promptVersion` it was installed from. Session and scheduled task IDs stay
  out of it: install and check look them up as [Install](#install) step 1
  says;
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
2. reads its `autonomy` from its declaration in that `main`, never from memory
   or an earlier run, and acts at those levels ([Autonomy](#autonomy));
3. records in `approvals` the maintainer's answers to its previous proposals,
   and every revert or correction of one of its `auto` actions as
   `corrected`; each answer is recorded once, by the first run that sees it;
4. does the job in its prompt file;
5. writes `status/<id>.json`, valid against
   [`status.schema.json`](../../agents/status.schema.json), commits and pushes
   it to `taxiway-sh/agents`;
6. ends with its report in the agent's session, where the maintainer reads and
   answers it; the heartbeat's `waiting` lists what needs them. Unattended
   reports are in English; when someone replies in the session, continue in
   their language.

A run that cannot finish still writes its heartbeat with `failed` or `partial`
and says why.

## Autonomy

A declaration's `autonomy` maps each action class the agent takes to a level.
A class missing from the map stays at `propose`. Levels apply within the
declared [repository access](#declarations).

| Level | The agent |
|---|---|
| `propose` | describes the action in its report; the maintainer acts |
| `ask` | proposes it in its report and takes it only after the maintainer's OK in its session; the initial level |
| `auto` | takes it, then reports it with a link that undoes it or shows how |

Each proposal and each `auto` action in a report carries an id,
`<id>-<UTC date YYYYMMDD>-<n>`, which the maintainer answers and `approvals`
records.

### Action classes

`make lint` rejects any other class and any level above a class's highest
level. A class whose highest level is `ask` is never `auto`, because it
changes the agentic system or is reserved to the maintainer by
[`AGENTS.md`](../../../AGENTS.md).

| Class | Covers | Highest level |
|---|---|---|
| `comment-issue` | Commenting on an issue or PR | `auto` |
| `create-issue` | Creating an issue through [`taxiway-intake`](../taxiway-intake/SKILL.md) | `auto` |
| `label-issue` | Applying or removing labels on an issue or PR | `auto` |
| `assign-issue` | Assigning or unassigning an issue or PR | `auto` |
| `review-pr` | Submitting a PR review | `auto` |
| `push-pr-branch` | Pushing, without force, to a PR branch the agent created, or to a bot PR branch opened by a Taxiway workflow (`automation/*`) | `auto` |
| `mark-pr-ready` | Marking a draft PR ready for review | `auto` |
| `merge-pr` | Merging a PR | `ask` |
| `close-issue` | Closing or reopening an issue | `auto` |
| `close-pr` | Closing or reopening a PR | `auto` |
| `rerun-workflow` | Rerunning a GitHub Actions run | `auto` |
| `write-state` | Writing its own files in `taxiway-sh/agents` other than its heartbeat, which every run writes | `auto` |
| `change-agent-system` | A draft PR changing skills, agent declarations or prompts, `AGENTS.md` or the agent docs checker | `ask` |

### Never

No level allows these actions and none is an action class: `make lint` rejects
them in a declaration. The checker holds the same ids and fails unless this
list matches them exactly. It also pins the never-`auto` classes by id, and
fails if one is missing from [Action classes](#action-classes) or has a highest
level above `ask`. It cannot check what an id covers, so changing a
description changes this list and needs the maintainer's review.

| Action | Never |
|---|---|
| `push-main` | Pushing to `main` of `taxiway-sh/taxiway` |
| `force-push-foreign` | Force-pushing a branch someone else created or pushed to |
| `publish-release` | Publishing a release ([`taxiway-release`](../taxiway-release/SKILL.md)) |
| `publish-upstream` | Upstream issues, PRs, comments, pushes or forks |
| `handle-secrets` | Reading, writing, printing or moving secret values |
| `skip-tests` | Skipping, disabling or quarantining a test or check |
| `change-own-instructions` | Changing its own declaration, prompt or the skills it uses |
| `change-approvals` | Changing the [approvals](../taxiway-workflow/SKILL.md#approvals) matrix or this Autonomy section |
| `change-never-list` | Changing this list or the checker that enforces it |

### Promotion and demotion

A level changes only through a `change-agent-system` PR that changes nothing
but the agent's declaration, reviewed and merged by the maintainer:

- **Promotion** to `auto`, for a class whose highest level is `auto`: after
  10 consecutive `ok` answers for that class and agent in `approvals`, with no
  `refused` or `corrected` answer among them.
- **Demotion** back to `ask`: after a revert or correction of an `auto` action
  of that class, recorded as `corrected`. The
  [watchdog](../../agents/watchdog.prompt.md) reports the incident at its next
  run.

The weekly improvement agent proposes these PRs once it is declared; until
then, an interactive session proposes them when the maintainer asks. Raising a
class from `propose` to `ask` is the maintainer's call, through the same
declaration-only PR.

## Install

For each declaration, or the one `id` requested:

1. Look up the agent's session and scheduled task, in this order, stopping at
   the first match:
   1. the session, not archived, tagged `taxiway-agent:<id>` (`list_sessions`
      with the tag filter, which an in-session caller may be refused), and
      the scheduled task bound to it;
   2. the scheduled task whose stored prompt is the bootstrap prompt naming
      `taxiway/.agents/agents/<id>.prompt.md`, paging through
      `list_triggers`; its `persistent_session_id` is the agent's session.
      This finds an agent whatever its names;
   3. the session and the scheduled task named with the declared `title`.

   Reuse them when they exist; never create a second one for the same agent.
   A session or scheduled task found under another name is renamed to the
   declared `title` (`set_session_title`, `update_trigger`).
2. Otherwise create one standing session for the agent from the `taxiway`
   repository, with the declared `model`, titled with the declared `title` and
   tagged `taxiway-agent:<id>`, and send a setup-only first prompt: attach
   `taxiway-sh/agents` with `add_repo` (`access: push`), verify both clones
   and the tools, then stop. Tools cannot be pre-approved from another
   session, so the maintainer approves that one `add_repo` call in the
   agent's session; the repository then stays attached as a source of the
   session.
3. Create a scheduled task named with the declared `title`, bound to that
   session, with the declared schedule and the bootstrap prompt. Never use a
   new session per run: such a session has no repository access.
4. When the schedule or the prompt changed, update the existing scheduled task.
   When the model changed, a scheduled task cannot be rebound to another
   session, so:
   1. create a new session as in step 2;
   2. create a new scheduled task bound to it as in step 3, with the same
      `title`, schedule and bootstrap prompt;
   3. delete the old scheduled task with `delete_trigger`;
   4. rename the old session `OLD session – <title>` with
      `set_session_title`, then archive it with `archive_session`, so title
      lookups find only the new one.

   The new scheduled task's run history starts empty.
5. Record the schedule, `model` and `promptVersion` in `deployed.json`.

Installation is idempotent: running it again changes nothing that already
matches the declarations.

## Check

Find each agent's session and scheduled task as in [Install](#install) step 1,
then report:

- the last heartbeat's date and result against the declared `schedule`: a
  heartbeat older than the latest time that cron expression fired, whether
  it fires weekly or several times a day, is late; late, failed or missing
  is a finding. Lateness comes from the declared cron, never from the
  scheduled task's last run. A run still in progress is running, not late;
- a declared agent that is missing from `deployed.json` and has no session
  or scheduled task: not installed, which is reported as such, not as a
  failed or late agent;
- the scheduled task: present, enabled, last run;
- the session: present and not failed;
- a session or scheduled task whose name is not the declared `title`;
- a `promptVersion` behind `main`, which shows the run read older
  instructions.

A check writes nothing. Repairs go through [Install](#install).
