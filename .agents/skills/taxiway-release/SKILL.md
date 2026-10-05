---
name: taxiway-release
description: Use when preparing, qualifying, cutting or resuming a Taxiway release, or checking whether a candidate is ready to publish.
---

# Taxiway release qualification

Durable E2E assertions enrich the nine existing scenarios in
`internal/cli/orchestrator_e2e_test.go`. Add helpers to that same file; do not
create another scenario file, standalone suite, `TestE2E_` entry point, CI job
or Makefile runner. `e2e_support_test.go` owns setup/cleanup, not a parallel suite.

Read `AGENTS.md` and the [release recipe](../../../docs/contributing/release-qualification.md).
That document owns checks, evidence and phase-specific commands. Use the same
source in Codex and Claude Code; no client-specific workflow engine is needed.

1. Reconcile the exact clean candidate, current agents/processes/resources and
   existing proof. Identify release scope, drivers/architectures and missing
   prerequisites. Follow `taxiway-workflow` for fixes and independent review.
2. Initialize a private report outside the checkout using
   `python3 tests/release/qualify.py --report <directory> init --candidate <SHA>`.
   Resume only the same clean HEAD. Inspect real running processes before retries;
   never duplicate paid calls or infer that silence means a test stopped.
3. Live qualification uses scoped manual checks with shared helpers, not a fixed
   committed feature suite. Preserve the real-account evidence requirements;
   controlled-provider E2Es do not prove real delegation or model availability.
   Work through the pre-publication table and documentation/behavior checklist.
   Reuse existing E2Es and live helpers. Coordinate one heavy run at a time;
   bound commands and provider usage. Authentication recovery may need one native
   user reconnection: distinguish auth from model/network failures, continue
   independent checks, and preserve credentials/unrelated labs.
4. Inspect actual test counts, skips, outputs/effects, observed versions/models,
   shipped assets and doc/help/code coherence. Audit historical compatibility/
   migration code and tests separately
   from necessary current recovery/idempotency. Review changes since the last
   published tag and prepare breaking-change notes with before/after, impact and
   user action; beta permits breaks and does not imply migration support. Commit
   markers help but never replace behavior review. Save sanitized exact-candidate
   proof. The helper's exit-zero PASS is provisional command evidence, not
   certification; external/manual records require independent inspection.
   Missing auth, skipped cases, timeouts, old-head E2Es and agent claims never
   count as validation. Leave unknown checks NOT_EXECUTED with pending actions.
   Consult Security Cloud privately before accepting review: establish scan date,
   SHA and coverage, assess intervening changes and triage findings. Unavailable
   or obsolete results leave review incomplete; an unresolved serious relevant
   vulnerability blocks publication. Keep findings, repros and screenshots out
   of public issues/PRs/notes/logs; publish only aggregate security status. An
   opened UI or empty response never proves that findings were reviewed.
5. Have a different reviewer inspect findings, fixes and proof; rerun affected
   checks on the final candidate. Present release blockers, qualified combinations,
   untested paths and the report, even under deadline pressure. Do not substitute
   core CI for the nine E2Es or snapshot success for published installation.
6. Qualification-only requests perform no tag, push or publication. A request
   to publish authorizes only the explicit requested publication after the recipe;
   follow `release.md` and do not push main. After authorized publication, start
   a separate `--phase post --candidate vX.Y.Z` report. Inspect all six installation
   matrix jobs and test installed archives on the actual Mac. Report failures;
   never silently overwrite an existing tag/release.

Clean only scenario-owned resources using existing helpers. Preserve reference
labs until their cleanup is authorized. Keep reports/logs/auth/recordings out of
Git. End with actual evidence, remaining checks and owned processes, and state
that monitoring stops with the session. Evaluator sessions, runner/oracles and
scientific analysis remain outside Taxiway qualification.
