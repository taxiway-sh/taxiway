# Contributing to Taxiway

Thanks for your interest in improving Taxiway. This is the quick entry point; the
full guide lives in [docs/contributing](docs/contributing/README.md).

## Getting started

- [Development](docs/contributing/development.md) — set up, build, and run Taxiway from a checkout.
- [Testing](docs/contributing/testing.md) — test categories and how to run them.

## Claim an issue

1. Pick an open issue without an assignee or an open PR; issues labeled
   [`good first issue`](https://github.com/taxiway-sh/taxiway/labels/good%20first%20issue)
   are a good start. Issues still labeled `needs-triage` may change scope, so
   ask first.
2. Comment on the issue that you would like to take it, and wait until it is
   assigned to you before starting, so two people do not do the same work.
3. Work against the issue's acceptance criteria and open one PR that fixes it.

## What to expect

- **Acknowledgement.** A new issue, claim or PR gets a first answer, usually
  within a few working days. Answers are in the language you wrote in.
- **Review.** Each PR gets a review against its linked issue: comments to
  address, or an approval. Some replies are drafted by an AI agent; reviews
  are approved by the maintainer before they are posted.
- **CI approval.** For first-time contributors, GitHub runs CI on a fork PR
  only after a maintainer approves it, so the first run may wait for that.
  Lifecycle [E2E scenarios](AGENTS.md#e2e-scenarios) do not run on PRs; the
  maintainer runs the affected ones before merging.
- **Follow-up.** If a PR or claim waits on you for two weeks, you get a
  friendly reminder; after another month without an answer, it may be closed
  with thanks and can be reopened at any time.
- **Credit.** Your commits keep you as author, and release notes thank each
  external contributor.

## Pull requests

- Changes land through pull requests. A PR may close one or several issues
  (`Fixes #NN`) and follows the
  [PR template](.github/pull_request_template.md).
- Commit messages follow
  [Conventional Commits](docs/contributing/development.md#commit-messages).

## Issues

- Open issues through the
  [issue templates](https://github.com/taxiway-sh/taxiway/issues/new/choose).
- See [how work is tracked](docs/contributing/issues.md) for the labels and the
  triage flow.

## Releases

- [Release process](docs/contributing/release.md).
