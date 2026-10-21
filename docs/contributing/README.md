# Contributing

How to set up, build, test, and track work on Taxiway.

| Page | Purpose |
|---|---|
| [Development](development.md) | Local setup, build, test, and commit conventions |
| [Testing](testing.md) | Test categories and how to run them |
| [Agent-driven live validation](live-tests.md) | Verify a feature in a real Docker or Lima lab, as a person would |
| [Model gateway validation](model-gateway-tests.md) | Gateway assertions and opt-in real-provider model checks |
| [Model catalog updates](model-catalog.md) | How the gateway model catalog update is prepared and reviewed |
| [Issues](issues.md) | How work is tracked: issue scope, labels, and triage |
| [Project agents](agentic-system.md) | Skills, scheduled agents, autonomy and safety of the agentic system |
| [Release](release.md) | The release process |
| [Release qualification recipe](release-qualification.md) | Qualify a release candidate before proposing a release |
| [Installation qualification](installation-qualification.md) | Verify published releases across platforms and drivers |

## At a glance

- Changes land through pull requests; a PR may close one or several issues
  (`Fixes #NN`).
- Commit messages follow [Conventional Commits](development.md#commit-messages).
- Open issues through the
  [issue templates](https://github.com/taxiway-sh/taxiway/issues/new/choose);
  see [Issues](issues.md) for the labels and triage flow.
