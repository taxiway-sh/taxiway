# AGENTS.md — taxiway

Taxiway is a local engineering lab for agent orchestrators: isolated Docker or
Lima labs, orchestrator adapters, local credentials, recorded terminal sessions
and local observability through Langfuse and LiteLLM. Keep it simple,
reproducible, observable and resettable; Kubernetes and heavyweight
infrastructure are out of scope. Repository `taxiway-sh/taxiway`, default branch `main`.

Each rule has one owner; link to it instead of restating it.

## Invariants

- Never push to `main`, even with permission to publish or merge: every change goes through a feature branch and PR.
- Merge, publish, force-push or publish upstream only when the user explicitly asks.
- Every `gh` command for this repository includes `--repo taxiway-sh/taxiway`.
- Beta: no historical compatibility or migration code/tests unless requested; keep current lifecycle, idempotency and recovery coverage.
- Conventional Commits, split by intention. An agent other than the implementer reviews each change.
- Prefer the existing code and documentation structure; no PRDs, ADRs, approval gates, run manifests or agent workflow docs unless requested.
- Before claiming work is complete, run the narrowest meaningful checks and report what passed or could not be run.
- Never run `gh auth login`, change GitHub authentication or git credential configuration, or set or print `GH_TOKEN`, `GITHUB_TOKEN`, `GIT_ASKPASS`, API keys, OAuth tokens, private keys or passwords; the local environment handles authentication.
- Never commit credentials, secrets, generated auth files, lab state, recordings, run logs, working plans, scratch files or private notes.

## E2E scenarios

Each orchestrator has three lifecycle E2E scenarios on Docker (Up, PrepareRun,
PhaseByPhase) in `internal/cli/orchestrator_e2e_test.go`. When a change affects
provisioned or runtime behavior, enrich the affected scenarios with assertions
and helpers in that file. Run them and verify they pass before merging; core
CI or skipped runs do not count. Adding an orchestrator adds its three
scenarios, Makefile targets and CI matrix entries. Do not add other parallel
suites, scenario files, `TestE2E_` entry points, jobs or runners;
`e2e_support_test.go` is setup/cleanup only.

## Need → read

| Need | Owner |
|---|---|
| Start or resume a session, approvals, coordination | [`taxiway-workflow` skill](.agents/skills/taxiway-workflow/SKILL.md) |
| New need, bug report, creating an issue; issue format, scope and labels | [`taxiway-intake` skill](.agents/skills/taxiway-intake/SKILL.md), [Issues](docs/contributing/issues.md) |
| Backlog ranking, label hygiene, external and bot PRs | [`taxiway-triage` skill](.agents/skills/taxiway-triage/SKILL.md) |
| Choosing and writing tests, E2E style, CI | [Testing](docs/contributing/testing.md) |
| Live validation on Docker or Lima, live authentication | [Live validation](docs/contributing/live-tests.md), [`tests/live/AGENTS.md`](tests/live/AGENTS.md), [model gateway](docs/contributing/model-gateway-tests.md) |
| Build, worktrees, PRs, commit messages, upstream dependency bugs, cleanup after an authorized merge | [Development](docs/contributing/development.md), [Post-merge cleanup](docs/contributing/development.md#post-merge-cleanup) |
| Releases and release qualification | [`taxiway-release` skill](.agents/skills/taxiway-release/SKILL.md) |
| Model catalog, installation qualification | [Model catalog](docs/contributing/model-catalog.md), [Installation qualification](docs/contributing/installation-qualification.md) |
| Where documentation lives | User docs in `docs/`, contributor docs in [`docs/contributing/`](docs/contributing/README.md) |
