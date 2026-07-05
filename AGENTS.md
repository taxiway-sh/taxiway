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
- Before claiming work is complete, run the narrowest meaningful checks and
  report what passed or could not be run.

## GitHub

- Repository: `taxiway-sh/taxiway`
- Default branch: `main`
- Every `gh` command for this repository must include
  `--repo taxiway-sh/taxiway`.
- Do not merge, publish, or force-push unless the user explicitly asks for it.

## Security

Do not:

- run `gh auth login` or change GitHub authentication;
- modify git credential configuration;
- set or print `GH_TOKEN`, `GITHUB_TOKEN`, `GIT_ASKPASS`, API keys, OAuth
  tokens, private keys, or passwords;
- commit credentials, secrets, generated auth files, lab state, or recordings.

Authentication is handled by the local environment.
