# Agent-driven live validation

Read [the live testing guide](../../docs/contributing/live-tests.md) before
creating an authenticated scenario. Which checks a change needs, Docker or Lima,
and cloud-session limits follow the
[`taxiway-testing` skill](../../.agents/skills/taxiway-testing/SKILL.md).

- Reuse `taxiway_live.py` for guest execution, auth propagation and temporary
  labs. Keep feature-specific scripts and sanitized results outside Git. This
  directory contains reusable tooling, not committed feature regression suites.
  Lifecycle nonregression assertions follow the
  [E2E scenario rule](../../.agents/skills/taxiway-testing/SKILL.md#e2e-scenarios).
- When a provider-free check fails, rerun it with `run --show-output` or call
  `diagnostic_output()` instead of bypassing the helper with raw `docker exec`
  or `limactl shell`
  ([diagnostic output](../../docs/contributing/live-tests.md#diagnostic-output-for-provider-free-runs)).
  It is refused for authenticated runs; never use it on commands that print
  credentials.
- Run with the worktree's dev/e2e environment loaded. Use bounded commands and
  short prompts. Real model calls consume account usage and are opt-in.
- Name persistent manual-validation labs `test-claude`, `test-codex` and
  `test-gastown` within the active worktree context. Reuse existing labs under
  their actual names; do not rename, recreate or replace them merely to adopt
  this convention. Keep persistent labs available for the user's manual checks
  and remove them only when requested. Temporary scenario names remain unique.
- Reuse the user's authenticated reference lab. Request interactive login only
  when the required login is missing or unusable; do not request it for every lab.
- Copy only Claude OAuth credentials through `propagate_claude_auth`; never
  print them, pass them in argv, or copy a gateway environment/key from another lab.
  The helper also copies completed first-run setup flags, preserving target
  preferences and workspace trust. Verify interactive readiness: successful
  `claude -p` calls alone do not prove the user can attach without onboarding.
  Codex authentication is managed by the gateway, not copied into lab clients;
  run `taxiway credentials codex` in the context before creating a Codex lab.
- Use `temporary_lab` for resources owned by a scenario. Preserve reference labs
  and other instances; never use global `destroy` or Docker prune for cleanup.
- Assert actual effects and client/gateway evidence. An agent's claim that it
  ran a tool or spawned a subagent is insufficient. Report skipped/blocked cases.
- Execute the task-specific check against real labs before claiming validation.
  Do not add a separate unit-test layer for scenario assertions.
