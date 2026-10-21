# Model catalog updates

The daily (23:37 UTC) and manually dispatched **Prepare model catalog update** workflow prepares one draft pull request on `automation/model-catalog`. It never merges changes or updates a running lab. Repository settings must allow GitHub Actions to create pull requests. No provider credentials are needed.

Run the same preparation locally:

```sh
go run ./cmd/model-catalog-update -report /tmp/model-catalog-review.md
go run ./cmd/model-catalog-update -write -report /tmp/model-catalog-review.md
```

Without `-write`, the command reports its proposal. With `-write`, it updates `infra/gateway/litellm/models.yaml` atomically only when model entries change. An unchanged run leaves the catalog byte-for-byte intact. Existing routing settings, defaults, comments, and supported old models are preserved. New entries inherit existing settings for their provider; the tool creates no new credential routes.

## Sources and review

The updater requires all six public sources: the [Codex structured catalog](https://github.com/openai/codex/blob/main/codex-rs/models-manager/models.json), [ChatGPT models](https://learn.chatgpt.com/docs/models), [Anthropic overview](https://platform.claude.com/docs/en/models/overview), [Anthropic deprecations](https://platform.claude.com/docs/en/about-claude/model-deprecations), [LiteLLM model metadata](https://github.com/BerriAI/litellm/blob/main/model_prices_and_context_window.json), and [LiteLLM releases](https://github.com/BerriAI/litellm/releases). Fetch, parse, schema, and incomplete-source errors produce no catalog update or PR. Source hashes and links appear in the review report.

Candidate selection, lifecycle rules and the review of the draft PR are owned by the [model catalog review](https://github.com/taxiway-sh/taxiway/blob/main/.agents/skills/taxiway-upstream/references/model-catalog.md) of the [`taxiway-upstream`](https://github.com/taxiway-sh/taxiway/blob/main/.agents/skills/taxiway-upstream/SKILL.md) skill.

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

No provider credentials are needed. In GitHub Actions, the updater uses the workflow token only for `https://api.github.com` to avoid shared-runner anonymous rate limits. Other source hosts receive no authentication header.
