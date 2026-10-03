# Reusable live tests

Live tests exercise an installed agent or a feature in real Taxiway labs. Use
them when a unit or offline protocol test cannot establish the behavior you
need, such as tool execution, authentication reuse, delegation or a restart.
They are opt-in: real model calls use the account's subscription allowance.

The shared helpers are in `tests/live/taxiway_live.py`. They use Python's
standard library and are independent of the model-selection scenarios.
Use Python 3.11 or newer on the host, a running Docker daemon for gateways,
and Docker or Lima for the labs. Run from a dev worktree with its `.envrc`
loaded through `direnv exec .`; all mutable runtime directories must be
below `TAXIWAY_RUNTIME_DIR`. The helper rejects the primary runtime context.

## Authenticate once

Build the local CLI to match the worktree assets, then create a reference lab:

```bash
go build -o taxiway ./cmd/taxiway
direnv exec . ./taxiway init
direnv exec . ./taxiway up live-auth --driver docker --type claude-code --skip-auth-check
direnv exec . ./taxiway auth live-auth claude-code
```

The user completes Claude's interactive login once. Keep `live-auth` for later
runs, or use an already authenticated lab in the same dev/e2e context instead.
No host credential file needs to be created or exported. A Lima reference lab
can supply credentials to Docker targets, and vice versa.

For each new target, the helper reads the reference lab's current
`~/.claude/.credentials.json`, validates the OAuth fields and copies it through
process memory and stdin. The target file is installed atomically with mode
`0600`, in a directory with mode `0700`. Tokens are not printed, passed in
command arguments, or written to an intermediate host file. Existing target
credentials are protected unless overwrite is explicitly requested.

Each target retains its own gateway key and environment. Only the Claude
login is copied. Normal client token refresh still applies; propagation does
not make an expired or revoked login valid. Re-authenticate the reference lab
if the client can no longer use or refresh its login, then propagate again.
The helper does not synchronize subsequent refreshes between running labs.

Codex works differently: Taxiway prepares ChatGPT authentication in the
gateway from the existing host Codex login. Do not copy Claude credentials or
host Codex auth files into Codex clients. API-key mode also uses gateway
credentials and is not handled by the Claude OAuth-copy helper.

## Scripts for developers and agents

To copy a reference login into an existing target lab:

```bash
direnv exec . python3 tests/live/taxiway_live.py auth --source live-auth --target my-test-lab
direnv exec . ./taxiway start my-test-lab
```

Add `--overwrite` only when replacing that target's existing login is intended.
The source and target must be different labs in the active context.

For a command in a new temporary authenticated lab:

```bash
direnv exec . python3 tests/live/taxiway_live.py run \
  --type claude-code --agent claude-code --auth-lab live-auth \
  --timeout 120 --expect LIVE_OK -- \
  claude -p 'Reply exactly LIVE_OK.' --tools '' --max-budget-usd 1
```

`run` creates a uniquely named lab, propagates the login, restarts its session,
runs the command with the lab's own gateway environment, then removes only that
temporary lab. Captured command output is not printed. A missing marker,
nonzero exit or timeout fails the smoke check and still triggers cleanup.
The reference login and shared Langfuse/proxy runtime remain available.

Use `--driver lima` for a Lima target, repeat `--set key=value` for orchestrator
settings, or add `--repo <git-url>` for a workspace. These settings are persisted
through the ordinary Taxiway CLI. For other orchestrators, use their `--type`;
`--agent claude-code` configures direct Claude commands and `--agent codex`
loads the common gateway environment for direct Codex commands. Commands that
operate on the orchestrator itself can omit `--agent`.

This is a smoke runner. A response marker alone is not proof of tool use,
delegation, or a particular model. Add the relevant assertions in a feature
scenario, as the [model gateway suites](model-gateway-tests.md) do.

## Add a scenario

Place a script under `tests/live/` and import these helpers:

| Helper | Purpose |
|---|---|
| `validate_context()` | Require an isolated dev/e2e environment and scoped runtime paths |
| `guest(lab, script, ...)` | Execute a guest shell script through the lab's persisted Docker/Lima driver; return captured bytes |
| `run_in_lab(lab, argv, agent=..., ...)` | Execute argv with the lab's gateway environment and a guest timeout |
| `propagate_claude_auth(source, target, overwrite=False)` | Reuse a reference OAuth login without copying gateway secrets |
| `temporary_lab(orch, auth_lab=..., ...)` | Create an owned lab and remove it on normal exit or an exception |
| `command(argv, ...)` | Run a bounded host command; capture output without printing it |
| `runtime_id(lab)` | Resolve the driver identifier from the active context |

For example, this scenario tests persistence across an orchestrator session
restart, independently of model selection:

```python
from taxiway_live import command, guest, temporary_lab

with temporary_lab("claude-code", auth_lab="live-auth") as lab:
    guest(lab, "printf 'fixture' > /lab/work/live-fixture")
    command(["./taxiway", "start", lab])
    output = guest(lab, '''
test -s "$HOME/.claude/.credentials.json" &&
test "$(cat /lab/work/live-fixture)" = fixture && printf RESTART_OK
''')
    assert output == b"RESTART_OK"
```

Add the assertions that establish your feature's behavior: inspect a resulting
file for an edit, client tool events for tool execution, child session metadata
for delegation, and gateway/Langfuse generations for actual model calls.
Reuse an existing suite when its assertions already cover your change.

`guest` captures shell stdout, so never print or persist credential-bearing
output in a scenario. Keep requests small and timeouts explicit. Tests run
sequentially unless their resource ownership and credential-refresh behavior
are known to support concurrency.

## Cleanup and failures

The context manager attempts removal even if setup or the scenario fails.
It never removes the reference lab or invokes runtime-wide destruction or
Docker prune. If cleanup fails, it reports the temporary lab name; remove that
specific lab with `taxiway rm <name> --yes`. A killed process or host crash
cannot guarantee cleanup: inspect `taxiway list` in that same context.

Missing authentication is a setup failure, not a passing or skipped live test.
Report the failing scenario and evidence, without dumping secrets or raw
request logs. Fix a protocol or lifecycle failure with a focused regression
test before rerunning the live scenario.

When the reference lab is no longer needed:

```bash
direnv exec . ./taxiway rm live-auth --yes
```
