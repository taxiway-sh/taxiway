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
  address, or an approval. Some replies and reviews are drafted by an AI
  agent; the maintainer approves them before they are posted.
- **CI approval.** GitHub runs CI on a fork PR only after a maintainer has
  read the changes and approved the run, so each run may wait for that. Your
  code runs only there, in fork-PR CI on GitHub-hosted runners, never on
  a workstation or in a Taxiway lab. Lifecycle
  [E2E scenarios](.agents/skills/taxiway-testing/SKILL.md#e2e-scenarios) do not run on PRs: if your change
  affects them, the maintainer may reimplement it on a Taxiway branch so they
  can run before merging, crediting you.
- **Follow-up.** If a PR or claim waits on you for two weeks, you get a
  friendly reminder. After another month without an answer, a claim is
  released so someone else can take it, and a PR may be closed with thanks;
  you can reopen it at any time.
- **Credit.** Your commits keep you as author. Merged contributions are
  thanked on the PR and credited in the notes of the release that ships them.

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
