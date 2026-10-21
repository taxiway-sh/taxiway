# Model catalog review

How to review the draft PR prepared by the **Prepare model catalog update**
workflow on `automation/model-catalog`, or a local run of
`cmd/model-catalog-update`. [Model catalog updates](../../../../docs/contributing/model-catalog.md)
documents the workflow, the commands, the public sources and offline
reproduction; this page owns the review.

## What the updater proposes

- **Sources.** All six public sources must be fetched and parsed; any fetch,
  parse, schema or incomplete-source error produces no catalog change and no PR.
  The report lists each source with its hash and link.
- **Codex candidates** are listed, accept text, have no specialty restriction,
  appear as an explicit model ID in the ChatGPT documentation, and have
  matching LiteLLM chat metadata.
- **Anthropic candidates** come from the overview's Claude API ID row and
  require matching LiteLLM metadata.
- **Excluded:** Mythos, preview, image or audio, and specialty models.
- New entries inherit the existing settings for their provider; the tool creates
  no new credential route and keeps routing settings, defaults, comments and
  supported old models.

These checks are public routing evidence. They do not prove access for every
account, plan or client, nor support in the lab's pinned LiteLLM release.
Public documents can lag a rollout, so skipped candidates need a manual look.

## Lifecycle rules

- Only an explicit lifecycle notice changes a model's status. Absence from a
  catalog, an old snapshot or an account's lack of access never establishes
  retirement.
- A "not sooner than" commitment is not a retirement date.
- Anthropic API notices affect `anthropic` entries. ChatGPT/Codex subscription
  notices affect only `chatgpt` entries; an OpenAI API retirement is a separate
  fact and is never inferred from a subscription retirement.
- A replacement ID is recorded only where the source names one unambiguously.

## Review

1. Read the report and the diff of `infra/gateway/litellm/models.yaml`; check
   each added, retired or replaced entry against the rules above and the
   linked sources.
2. Check compatibility with the pinned LiteLLM gateway version
   ([Where versions live](../SKILL.md#where-versions-live)).
3. A retired model that is still a default is preserved and flagged by the
   tool, and validation may fail until a reviewer chooses the replacement:
   choose it explicitly, in the catalog and in the orchestrator manifest
   defaults that name it.
4. The workflow runs its own checks (build, unit and script tests, formatting,
   script lint, gateway protocol tests) because a PR created with
   `GITHUB_TOKEN` triggers no ordinary PR CI, and records each result in the
   PR. A failed check is a finding: read the step's log and find its root
   cause before proposing a fix.
5. The tool never marks the PR ready or merges it; neither does the reviewer
   without the maintainer's approval.

## Output

1. **Status.** The draft PR's link and state, the date of the last run, and
   each recorded check with its result; for a failure, the failing step and
   its root cause or the open question.
2. **Changes.** One line per added, retired or replaced model, with the source
   that justifies it, and any rule above it breaks.
3. **Defaults.** Each default the change touches, with the proposed
   replacement and why.
4. **Proposals.** Fixes to the updater or the catalog, worked through
   [`taxiway-issue`](../../taxiway-issue/SKILL.md), or a tracking issue through
   [`taxiway-intake`](../../taxiway-intake/SKILL.md) when the cause is not in
   this PR.
5. **Needs the maintainer.** Marking the PR ready, pushing to its branch,
   choosing a replacement default, merging, closing it or creating an issue,
   each with its link ([approvals](../../taxiway-workflow/SKILL.md#approvals)).
