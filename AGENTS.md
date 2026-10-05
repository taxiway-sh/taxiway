# AGENTS.md — taxiway

This file is the lightweight working contract for agents contributing to
Taxiway. Keep it small and practical. Public documentation lives under `docs/`.

## Project

Taxiway is a local engineering lab for agent orchestrators. It creates isolated
Docker or Lima labs, runs orchestrator adapters, provisions local credentials,
records terminal sessions, and exposes local observability through Langfuse and
LiteLLM.

Keep the project:

- simple;
- reproducible;
- observable;
- resettable.

Kubernetes and heavyweight infrastructure are out of scope.

## Working Rules

- Prefer the existing code and documentation structure over new process.
- Use a dedicated worktree and implementation agent per subject, and a different
  agent for independent review. Follow the [contribution workflow](docs/contributing/development.md#contribution-workflow).
- Taxiway is in beta: do not add historical compatibility or migration code/tests
  unless requested. Keep current lifecycle, idempotency and recovery coverage.
- Test product behavior directly; do not add a separate test layer for test assertions.
- When a feature changes provisioned or runtime behavior, enrich the relevant
  existing E2E scenarios with assertions of that behavior. Run them and verify
  they pass before merging; core CI or skipped E2E tests are not sufficient.
- Do not add PRDs, ADRs, approval gates, run manifests, or agent workflow docs
  unless explicitly requested.
- Do not commit working plans, scratch files, local run logs, generated lab
  state, or private notes.
- User-facing documentation belongs in `docs/`.
- Contributor documentation belongs in `docs/contributing/`.
- Use Conventional Commits so GoReleaser can generate useful release notes:
  - `feat:` for user-facing capabilities;
  - `fix:` for bug fixes;
  - `cleanup:` for simplification or removal work;
  - `docs:`, `test:`, `refactor:`, and `chore:` where appropriate.
- Split commits by intention; PRs describe behavior, validation and limitations,
  link resolved issues with `Fixes #...`, and use existing appropriate labels.
- Before claiming work is complete, run the narrowest meaningful checks and
  report what passed or could not be run.
- Live validation is a task-specific manual check by an agent, not a committed
  feature regression suite. Reuse `tests/live/taxiway_live.py` and read
  `tests/live/AGENTS.md` plus `docs/contributing/live-tests.md`. Keep its scripts
  and sanitized evidence temporary; durable behavior assertions belong in E2E.
- For release requests, use the shared `taxiway-release` skill and
  [release recipe](docs/contributing/release-qualification.md); qualification
  does not authorize publication, and missing/skipped checks are not validation.

## GitHub

- Repository: `taxiway-sh/taxiway`
- Default branch: `main`
- Every `gh` command for this repository must include
  `--repo taxiway-sh/taxiway`.
- Do not merge, publish, or force-push unless the user explicitly asks for it.
- Never push directly to `main`. All changes, including documentation and
  agent instructions, must go through a feature branch and pull request.
- After an authorized PR merge, clean up its Taxiway instance, labs, worktree,
  and feature branch without further confirmation. Update `main` and active
  branches while preserving ongoing work and other instances; follow
  [post-merge cleanup](docs/contributing/development.md#post-merge-cleanup).

## Security

Do not:

- run `gh auth login` or change GitHub authentication;
- modify git credential configuration;
- set or print `GH_TOKEN`, `GITHUB_TOKEN`, `GIT_ASKPASS`, API keys, OAuth
  tokens, private keys, or passwords;
- commit credentials, secrets, generated auth files, lab state, or recordings.

Authentication is handled by the local environment.
