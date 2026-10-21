# Issues

How work is tracked in this repository. Agents apply these rules through the
[`taxiway-intake`](https://github.com/taxiway-sh/taxiway/blob/main/.agents/skills/taxiway-intake/SKILL.md)
and [`taxiway-triage`](https://github.com/taxiway-sh/taxiway/blob/main/.agents/skills/taxiway-triage/SKILL.md)
skills.

## An issue is a report; the PR is the unit of work

An issue describes a problem or a request. The actual change happens in a pull
request, and the two do not have to map one-to-one:

- one PR may close several issues (`Fixes #48, #49`);
- one large issue may be closed by several PRs;
- a duplicate is not a problem to avoid but a state to resolve — the
  best-described issue is kept as canonical and the others are closed as
  duplicates with a link.

Because of this, the exact size of an issue matters less than its coherence.

## Scope of an issue

Aim for **one coherent, independently-mergeable change** per issue (roughly one
PR):

- **Group** things that share a root cause or the same files into one issue, and
  use a task list (`- [ ]`) inside it for visibility.
- **Split** only when the items are unrelated subjects, or mix a bug with a new
  capability, or live in different files and ship independently.
- Do not create one-line issues just for uniformity — atomic is fine when it
  falls out naturally, not when forced.
- Independent documentation fixes are the exception worth splitting: each small,
  self-contained correction makes a good `good first issue`.
- An issue is about the Taxiway product or how to contribute to it. The
  maintainer's personal tooling, agent setup and machine, and studies of
  them, are not Taxiway work and stay out of issues, PRs and comments;
  approved Taxiway studies and plans are posted as described in
  [`taxiway-workflow`](https://github.com/taxiway-sh/taxiway/blob/main/.agents/skills/taxiway-workflow/SKILL.md).
  Host details and private paths follow the
  [security rules](https://github.com/taxiway-sh/taxiway/blob/main/AGENTS.md#security).

## Issue format

Open issues through the [issue templates](https://github.com/taxiway-sh/taxiway/issues/new/choose).
They follow a common shape so issues stay comparable:

- **Problem** — the symptom and, where known, the root cause.
- **Current behavior** *(optional)* — reproduction, logs, observed failure.
- **Expected behavior** — the contract once fixed.
- **Possible directions** *(optional)* — leads, kept open and unranked. An issue
  records a preferred option only once the maintainer has decided it
  (see the intake *Decisions* step).
- **Acceptance criteria** — verifiable conditions, ideally including a test.

An issue centers on the problem or the need, with evidence; it carries no
session narrative and no release-status remarks such as "not blocking the
release". New facts enrich the existing issue instead of opening a duplicate.
A defect found in delivered work gets a new issue that links the closed one;
the closed issue is neither reopened nor commented on.

Reference real commands, files, and symbols. Security-sensitive reports follow
[public writing](https://github.com/taxiway-sh/taxiway/blob/main/.agents/skills/taxiway-security/SKILL.md#public-writing).

## Labels

| Axis | Values | Set by |
|------|--------|--------|
| **type** | `bug` \| `enhancement` \| `documentation` | reporter (via template), confirmed at triage |
| **priority** | `priority:p0` \| `priority:p1` \| `priority:p2` | maintainer at triage |
| **area** | `capability:*`, `driver:*`, `orchestrator:*`, `agent:*` | maintainer at triage |
| **intake** | `needs-triage` | applied by the templates; removal per [conventions](#label-conventions) |
| **onboarding** | `good first issue` | maintainer |

Type means:

- **bug** — remediation of the existing product: broken behavior, a fragility to
  harden, refactoring, technical debt, CI/test infrastructure, an outdated
  default, or a clarified contract.
- **enhancement** — a genuinely new product capability that does not exist today.
- **documentation** — documentation-only changes.

Priority means: **p0** incorrect behavior or data loss · **p1** inconsistency,
fragility, near-term · **p2** cleanliness or later.

### Label conventions

- The type is the label: the repository does not use GitHub issue types.
- The label set is intentionally small. Please do not introduce new label axes;
  a distinction can almost always be expressed with type × priority × area.
  Each issue carries one type, one priority and only the area labels that
  locate the change.
- Adding an orchestrator creates its `orchestrator:<name>` label: it extends an
  existing axis rather than adding one.
- `needs-triage` is removed at triage, or as soon as work on the issue starts.
- GitHub's default `duplicate`, `invalid`, `question`, `wontfix` and
  `help wanted` labels are accepted as resolution and onboarding markers, like
  `good first issue`; they are not area axes.
- `Security` marks a security-sensitive issue, in addition to its type,
  priority and area labels; write it as described in [Issue format](#issue-format).

## What happens after you open an issue

A maintainer triages each new issue: confirms the type, sets a priority and
area, links or closes duplicates, and either accepts it, asks for more
information, or closes it. Reporters' issues are triaged and linked, not
rewritten.

Larger efforts are organized with milestones (release waves) and sub-issues
(for epics), rather than by reshaping the granularity of individual issues.
