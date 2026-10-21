---
name: check-agents
description: Use when asked for the state of Taxiway's cloud scheduled agents, or when invoked as /check-agents.
argument-hint: "[id]"
---

# Check agents

Reports, per declared cloud agent, its heartbeat, session and scheduled task,
and old sessions waiting for archive. Writes nothing.

Argument, optional: an agent `id` (default: every agent).

Follow [`taxiway-agents` Check](../taxiway-agents/SKILL.md#check).
