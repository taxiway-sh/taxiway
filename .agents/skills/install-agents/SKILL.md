---
name: install-agents
description: Use when asked to install, update, reinstall or remove Taxiway's cloud scheduled agents, or when invoked as /install-agents.
argument-hint: "[id] [reinstall|remove]"
---

# Install agents

Installs or updates the declared cloud agents, reinstalls them, or removes
agents no longer declared, after one maintainer OK on the plan.

Arguments, both optional: an agent `id` (default: every agent), then a mode,
`reinstall` or `remove` (default: install or update).

Follow [`taxiway-agents` Install](../taxiway-agents/SKILL.md#install).
