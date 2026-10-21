---
name: taxiway-release
description: Use when preparing, qualifying, cutting or resuming a Taxiway release, or checking whether a candidate is ready to publish.
---

# Taxiway release qualification

Read [`AGENTS.md`](../../../AGENTS.md) and the
[release recipe](../../../docs/contributing/release-qualification.md); durable
E2E assertions follow the [E2E scenario rule](../taxiway-testing/SKILL.md#e2e-scenarios).
The recipe owns checks, evidence and phase-specific commands. Use the same
source in Codex and Claude Code; no client-specific workflow engine is needed.

Cut a release only when the CLI, installer, packaging or runtime changed since
the last published tag (the recipe's `breaking-changes` base tag);
documentation-only and agent-tooling changes wait for the next one.

1. Reconcile the exact clean candidate, current agents/processes/resources and
   existing proof. Identify release scope, drivers/architectures and missing
   prerequisites. Fix through [`taxiway-issue`](../taxiway-issue/SKILL.md) and review with [`taxiway-review`](../taxiway-review/SKILL.md).
2. Initialize a private report outside the checkout using
   `python3 tests/release/qualify.py --report <directory> init --candidate <SHA>`.
   Resume only the same clean HEAD. Inspect real running processes before retries;
   never duplicate paid calls or infer that silence means a test stopped.
3. Live qualification uses scoped manual checks with shared helpers, not a fixed
   committed feature suite. Preserve the real-account evidence requirements;
   controlled-provider E2Es do not prove real delegation or model availability.
   Work through the pre-publication table and documentation/behavior checklist.
   Reuse the live helpers. Coordinate one heavy run at a time;
   bound commands and provider usage. Authentication failures follow
   [When authentication fails](../../../docs/contributing/live-tests.md#when-authentication-fails).
4. Inspect actual test counts, skips, outputs/effects, observed versions/models
   (against the pins in [`taxiway-upstream`](../taxiway-upstream/SKILL.md#where-versions-live)),
   shipped assets and doc/help/code coherence. Audit historical compatibility/
   migration code and tests against the [beta invariant](../../../AGENTS.md#invariants),
   separately from necessary current recovery/idempotency. Review changes since the last
   published tag and prepare breaking-change notes with before/after, impact and
   user action; beta permits breaks and does not imply migration support. As
   part of the authorized publication, add these prepared notes and the
   [contributor credit](../taxiway-contributions/SKILL.md#credit) to the
   published release with `gh release edit`. Commit
   markers help but never replace behavior review. Save sanitized exact-candidate
   proof. The helper's exit-zero PASS is provisional command evidence, not
   certification; external/manual records require independent inspection.
   Missing auth, skipped cases, timeouts, old-head E2Es and agent claims never
   count as validation. Leave unknown checks NOT_EXECUTED with pending actions.
   Run the [private security review](../taxiway-security/SKILL.md#private-security-review)
   before accepting review.
5. Have a different reviewer inspect findings, fixes and proof; rerun affected
   checks on the final candidate. Present release blockers, qualified combinations,
   untested paths and the report, even under deadline pressure, and end with
   one go/no-go line that names deferred items and untested combinations.
   Lifecycle E2E coverage follows the
   [E2E scenario rule](../taxiway-testing/SKILL.md#e2e-scenarios), and snapshot
   success does not replace published installation.
6. Qualification-only requests perform no tag, push or publication. A request
   to publish authorizes only the explicit requested publication after the recipe;
   follow `release.md`. After authorized publication, start
   a separate `--phase post --candidate vX.Y.Z` report. Inspect all six installation
   matrix jobs and test installed archives on a real macOS machine. Report failures;
   never silently overwrite an existing tag/release.

Clean only scenario-owned resources using existing helpers. Preserve reference
labs until their cleanup is authorized. End with actual evidence, remaining
checks and owned processes, and state
that monitoring stops with the session. Evaluator sessions, runner/oracles and
scientific analysis remain outside Taxiway qualification.
