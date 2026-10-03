# Live Taxiway tests

Read [the live testing guide](../../docs/contributing/live-tests.md) before
creating an authenticated scenario.

- Reuse `taxiway_live.py` for guest execution, auth propagation and temporary
  labs. Keep feature-specific assertions in the scenario that tests the feature.
- Run with the worktree's dev/e2e environment loaded. Use bounded commands and
  short prompts. Real model calls consume account usage and are opt-in.
- Reuse the user's authenticated reference lab. Request interactive login only
  when the required login is missing or unusable; do not request it for every lab.
- Copy only Claude OAuth credentials through `propagate_claude_auth`; never
  print them, pass them in argv, or copy a gateway environment/key from another lab.
  Codex authentication is managed by the gateway, not copied into lab clients.
- Use `temporary_lab` for resources owned by a scenario. Preserve reference labs
  and other instances; never use global `destroy` or Docker prune for cleanup.
- Assert actual effects and client/gateway evidence. An agent's claim that it
  ran a tool or spawned a subagent is insufficient. Report skipped/blocked cases.
- Run the changed scenario against real labs before claiming live coverage.
  Do not add a separate unit-test layer for scenario assertions.
