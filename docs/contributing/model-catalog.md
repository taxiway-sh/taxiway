# Model catalog updates

The daily and manually dispatched **Prepare model catalog update** workflow prepares one draft pull request on `automation/model-catalog`. It never merges changes or updates a running lab. Repository settings must allow GitHub Actions to create pull requests. No provider credentials are needed.

Run the same preparation locally:

```sh
go run ./cmd/model-catalog-update -report /tmp/model-catalog-review.md
go run ./cmd/model-catalog-update -write -report /tmp/model-catalog-review.md
```

Without `-write`, the command reports its proposal. With `-write`, it updates `infra/gateway/litellm/models.yaml` atomically only when model entries change. An unchanged run leaves the catalog byte-for-byte intact. Existing routing settings, defaults, comments, and supported old models are preserved. New entries inherit existing settings for their provider; the tool creates no new credential routes.

## Evidence and review

The updater requires all six public sources: the [Codex structured catalog](https://github.com/openai/codex/blob/main/codex-rs/models-manager/models.json), [ChatGPT models](https://learn.chatgpt.com/docs/models), [Anthropic overview](https://platform.claude.com/docs/en/models/overview), [Anthropic deprecations](https://platform.claude.com/docs/en/about-claude/model-deprecations), [LiteLLM model metadata](https://github.com/BerriAI/litellm/blob/main/model_prices_and_context_window.json), and [LiteLLM releases](https://github.com/BerriAI/litellm/releases). Fetch, parse, schema, and incomplete-source errors produce no catalog update or PR. Source hashes and links appear in the review report.

Codex candidates must be listed, accept text, have no specialty restriction, appear as an explicit model ID in ChatGPT documentation, and have matching LiteLLM chat metadata. Anthropic candidates come from the overview's Claude API ID row and require matching LiteLLM metadata. Mythos, preview, image/audio, and specialty models are excluded. Public documents can lag a rollout: skipped candidates require manual review. These checks provide public routing evidence; they do not prove access for every account, plan, client, or the lab's pinned LiteLLM release.

Only explicit lifecycle notices change model status. Catalog absence, old snapshots, and account access do not establish retirement. “Not sooner than” commitments are not retirement dates. Anthropic API notices affect `anthropic` entries. ChatGPT/Codex subscription notices affect only `chatgpt` entries; OpenAI API retirement is a separate fact and must never be inferred from a subscription retirement. Replacement IDs are recorded only where the source names one unambiguously.

Review the proposed diff and report, validate compatibility with the pinned gateway version, and choose replacement defaults explicitly when required. The tool preserves default choices even when a source announces retirement, flags them for review, and exposes the change in a draft PR. Such proposals may fail validation until a reviewer changes the default. Orchestrator manifest defaults also need review. The workflow runs build, unit tests, script tests, formatting, script lint, and isolated gateway protocol tests directly because a PR created using `GITHUB_TOKEN` does not trigger ordinary PR CI. It records each result in the PR and fails visibly after preparing review if any check fails. It never marks the PR ready or auto-merges.

## Offline reproduction

Place a complete source set in a directory using these filenames:

```text
codex.txt
chatgpt.txt
anthropic.txt
deprecations.txt
litellm.txt
releases.txt
```

JSON sources retain their public JSON schema; documentation sources use their Markdown endpoint (`.md`). All fixture files use the same strict parsers as online data. Pass `-fixtures /path/to/sources -as-of 2026-10-02` to reproduce retirement calculations at a chosen UTC date. Reports identify the canonical sources and hash the supplied data; fixtures do not prove current public availability. Do not commit downloaded snapshots, local reports, or generated lab state.

```sh
go test ./internal/modelupdate ./cmd/model-catalog-update
```
