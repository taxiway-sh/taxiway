# Issues

How work is tracked in this repository.

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

## Issue format

Open issues through the [issue templates](https://github.com/taxiway-sh/taxiway/issues/new/choose).
They follow a common shape so issues stay comparable:

- **Problem** — the symptom and, where known, the root cause.
- **Current behavior** *(optional)* — reproduction, logs, observed failure.
- **Expected behavior** — the contract once fixed.
- **Proposed direction** *(optional)* — a lead, kept open.
- **Acceptance criteria** — verifiable conditions, ideally including a test.

Reference real commands, files, and symbols. Avoid step-by-step exploitation
details for security-sensitive reports — describe what to harden and the impact.

## Labels

| Axis | Values | Set by |
|------|--------|--------|
| **type** | `bug` \| `enhancement` \| `documentation` | reporter (via template), confirmed at triage |
| **priority** | `priority:p0` \| `priority:p1` \| `priority:p2` | maintainer at triage |
| **area** | `capability:*`, `driver:*`, `orchestrator:*`, `agent:*` | maintainer at triage |
| **intake** | `needs-triage` | applied automatically, removed at triage |
| **onboarding** | `good first issue` | maintainer |

Type means:

- **bug** — remediation of the existing product: broken behavior, a fragility to
  harden, refactoring, technical debt, CI/test infrastructure, an outdated
  default, or a clarified contract.
- **enhancement** — a genuinely new product capability that does not exist today.
- **documentation** — documentation-only changes.

Priority means: **p0** incorrect behavior or data loss · **p1** inconsistency,
fragility, near-term · **p2** cleanliness or later.

The label set is intentionally small. Please do not introduce new label axes; a
distinction can almost always be expressed with type × priority × area.

## What happens after you open an issue

New issues arrive with `needs-triage`. A maintainer then confirms the type, sets
a priority and area, links or closes duplicates, and either accepts it, asks for
more information, or closes it. Reporters' issues are triaged and linked, not
rewritten.

Larger efforts are organized with milestones (release waves) and sub-issues
(for epics), rather than by reshaping the granularity of individual issues.
