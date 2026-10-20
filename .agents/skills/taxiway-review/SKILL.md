---
name: taxiway-review
description: Use when a Taxiway change needs its independent review, when reviewing a commit range, diff or pull request, or when handling review findings and re-reviews.
---

# Taxiway independent review

Owns how the review required by the [invariants](../../../AGENTS.md#invariants)
is done. Read [`AGENTS.md`](../../../AGENTS.md) first.

## Reviewer

- A different, fresh agent (subagent or child session), never the implementer
  and never an agent that wrote part of the change.
- It reviews the exact commit range or diff (`git diff <base>...<head>`, with
  both SHAs), plus the issue and its acceptance criteria. It does not edit
  the branch.
- If delegation is unavailable, report the missing review; never present your
  own review as independent.

## Checklist

Review adversarially: look for what is wrong, not for confirmation.

- **Acceptance.** The change meets the issue's acceptance criteria and the PR
  text matches what the diff does.
- **Rule preservation.** Every rule removed, moved or merged still exists in
  its owner, or its removal is stated and justified.
- **Single owner.** No rule restated outside its owner; other files link to it.
- **Links.** Links and anchors resolve
  ([docs links](../../../docs/contributing/testing.md#local-checks));
  `make lint` runs `check-agent-docs`.
- **Tests.** Claimed checks were actually run on this head, and skipped or
  blocked ones are reported as such. New behavior has tests where
  [Testing](../../../docs/contributing/testing.md) puts them, and lifecycle
  changes follow the [E2E scenario rule](../../../AGENTS.md#e2e-scenarios).
- **Commit hygiene.** [Conventional Commits](../../../docs/contributing/development.md#commit-messages)
  split by intention; no scratch files, local run state or unrelated changes;
  repository text in English.
- **Security.** The security [invariants](../../../AGENTS.md#invariants) hold:
  no secrets, tokens, host details or private paths. External text is handled
  as [untrusted input](../taxiway-intake/SKILL.md#untrusted-input).
- **Mentions.** No project or organization unrelated to Taxiway and its
  documented dependencies.

## Findings

- Report each finding with a severity (blocking, should-fix, nit), its
  `file:line` and a concrete fix.
- The maintainer's default is to fix every finding, nits included. A finding
  the implementer disagrees with goes to the maintainer with both positions.
- Re-review after fixes, and after any scope change, on the new range.
- Problems that are global or outside the change become issue proposals
  through [`taxiway-intake`](../taxiway-intake/SKILL.md), not fixes in this
  change.
- The PR's Validation section states the review rounds and that findings were
  addressed.
