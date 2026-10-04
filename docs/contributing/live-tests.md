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
direnv exec . ./taxiway up test-claude --driver docker --type claude-code --skip-auth-check
direnv exec . ./taxiway auth test-claude claude-code
```

The user completes Claude's interactive login once. Keep `test-claude` for later
runs, or use an already authenticated lab in the same dev/e2e context instead.
No host credential file needs to be created or exported. A Lima reference lab
can supply credentials to Docker targets, and vice versa.

For each new target, the helper reads the reference lab's current
`~/.claude/.credentials.json`, validates the OAuth fields and copies it through
process memory and stdin. The target file is installed atomically with mode
`0600`, in a directory with mode `0700`. Tokens are not printed, passed in
command arguments, or written to an intermediate host file. Existing target
credentials are protected unless overwrite is explicitly requested.

The helper also merges `hasCompletedOnboarding` and `lastOnboardingVersion`
from the reference into the target’s `~/.claude.json`. It preserves the target’s
account metadata, theme, project trust and gateway settings; it never copies the
whole reference configuration. The reference must have completed interactive
onboarding, so authenticated noninteractive calls alone are not sufficient.

Each target retains its own gateway key and environment. Only the Claude
login and the completed first-run setup flags are copied.
Before provisioning each authenticated scenario, and immediately before copying
its login, the helper runs a native Claude Haiku request in the reference. It
allows native OAuth refresh, removes gateway/API-key environment overrides,
disables tools and MCP servers, and bounds the request to 90 seconds and a $0.10
budget. Only a sanitized result category leaves the guest; response/error text
is withheld. This uses subscription allowance even if the scenario never starts.

To verify a reference without creating any scenario lab:

```bash
direnv exec . python3 tests/live/taxiway_live.py preflight --source test-claude
```

Missing or explicitly rejected authentication stops before scenario provisioning
and prints the exact native `claude auth login` command for the reference's
Docker or Lima driver. Complete that command once, then rerun preflight or the
failed scenario: the renewed reference is verified before credentials are
copied. `taxiway auth` can short-circuit on an existing file and is not a renewal
command. Network/timeouts, unavailable models, setup and other provider failures
do not request login. Diagnose them separately; unknown errors are not treated
as proof of rejected authentication.

The reference must also have completed interactive onboarding; the helper checks
that before provisioning. Native login alone may not complete it. Open `claude`
interactively in the reference, finish its first-run prompts, then exit. This is
a setup step, not another authentication request.

If auth expires during a suite, the failing action is not replayed automatically.
The next propagation runs another bounded preflight; after confirmed rejection,
reconnect once and explicitly rerun the failed scenario. There are no infinite
refresh/login/setup retries, and temporary-lab cleanup preserves the reference.
Native refresh success is established by a successful real request, never by
credential-file presence.
The helper does not synchronize subsequent refreshes between running labs.

Codex works differently: Taxiway prepares ChatGPT authentication in the
gateway from the existing host Codex login. Do not copy Claude credentials or
host Codex auth files into Codex clients. API-key mode also uses gateway
credentials and is not handled by the Claude OAuth-copy helper.

## Authentication responsibilities

The shared tooling reuses authentication after the initial user login:

| Agent | Initial authentication | Reuse by tests |
|---|---|---|
| Claude Code | User logs in once inside a reference lab such as `test-claude` | `temporary_lab(..., auth_lab=...)` copies OAuth credentials before starting roles; the `auth` command handles persistent targets |
| Gas Town | Uses the same Claude Code reference login | Same copy helper; each Gas Town lab keeps its own gateway configuration |
| Codex | User has an existing Codex subscription login on the host | Taxiway prepares the lab gateway's ChatGPT authentication cache; no login is copied into the lab client |

Pass the actual Claude reference name with `--auth-lab` (or `--source` for
persistent targets). The helper does not guess a reference lab or complete
interactive login. Initial, expired or revoked authentication still requires
user intervention when the client cannot refresh it. API-key authentication
is managed through the ordinary gateway credentials, outside the OAuth-copy
helper. No credentials are committed to the repository.

## Persistent labs for manual validation

Use these feature-independent names within each dev worktree:

| Lab | Orchestrator | Purpose |
|---|---|---|
| `test-claude` | `claude-code` | Manual Claude checks and reusable reference authentication |
| `test-codex` | `codex` | Manual Codex checks using host authentication through the gateway |
| `test-gastown` | `gastown` | Manual Gas Town checks using a copy of the reference Claude login |

The runtime context isolates these names between worktrees. Existing labs may
have older names: pass their actual names to the scripts and keep using them;
updating this naming convention does not rename or recreate any resources.

After authenticating `test-claude` above, create the other labs when validation
of the feature needs them:

```bash
direnv exec . ./taxiway up test-codex --driver docker --type codex
direnv exec . ./taxiway up test-gastown --driver docker --type gastown \
  --repo https://github.com/octocat/Hello-World --prepare-only --skip-auth-check
direnv exec . python3 tests/live/taxiway_live.py auth --source test-claude --target test-gastown
direnv exec . ./taxiway run test-gastown
```

Prepare Gas Town before copying authentication so its roles first start with
a usable login. If a lab already exists, inspect and reuse it instead of
creating a replacement or overwriting its authentication automatically.

Leave persistent labs available after automated checks for the user's manual
validation. Connect with `direnv exec . ./taxiway shell <lab>`; in Gas Town,
use `gt status` and `gt mayor attach`. Remove persistent labs only when their
validation work is finished and removal is requested.

Automatically owned scenarios continue to use unique `live-test-<random>`
names and clean up their own labs. A temporary scenario never removes one of
the three persistent labs.

## Scripts for developers and agents

To copy a reference login into an existing target lab:

```bash
direnv exec . python3 tests/live/taxiway_live.py auth --source test-claude --target my-test-lab
direnv exec . ./taxiway start my-test-lab
```

Add `--overwrite` only when replacing that target's existing login is intended.
The source and target must be different labs in the active context.

For a command in a new temporary authenticated lab:

```bash
direnv exec . python3 tests/live/taxiway_live.py run \
  --type claude-code --agent claude-code --auth-lab test-claude \
  --timeout 120 --expect LIVE_OK -- \
  claude -p 'Reply exactly LIVE_OK.' --tools '' --max-budget-usd 1
```

`run` prepares a uniquely named lab, propagates the login before starting any
agent roles, runs the ordinary runtime phases,
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

## Gas Town lifecycle and delegation

With an authenticated reference lab, run:

```bash
direnv exec . python3 tests/live/test_gastown.py --auth-lab test-claude
```

The scenario creates a temporary Docker Gas Town lab with a small public
repository (`octocat/Hello-World`, override with `--repo`). It verifies the rig
and crew workspace, checks running Claude roles and their gateway/model/alias
environment, then verifies that Deacon completes a real patrol and advances
its heartbeat. A real Deacon handoff must replace the Claude process and
complete another patrol with new checks. A second fresh lab independently
verifies autonomous patrol startup; self-sling interruptions fail the scenario
even if the daemon later recovers. The suite also exercises the Mayor's real
`gt handoff` restart. It stops
background patrols before a bounded inference through the Gas Town launcher:
an Opus principal invokes a Sonnet subagent and writes a proof file using Bash.
Assertions inspect actual child responses, tool events and the resulting file.
The temporary lab is removed and reference authentication is preserved.

This is real account usage, including the short period of Gas Town startup
and handoff before patrols are stopped. It does not validate a full polecat,
refinery or merge-queue workload.

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

with temporary_lab("claude-code", auth_lab="test-claude") as lab:
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

When removal of the reference lab is requested after validation:

```bash
direnv exec . ./taxiway rm test-claude --yes
```

## Guest capabilities

Taxiway orchestrators default to Full Access/YOLO inside their guests. Codex
writes `approval_policy = "never"` and `sandbox_mode = "danger-full-access"`
to its guest configuration, so fresh sessions, resume and delegated agents have
both approval bypass and an unrestricted client sandbox. Fresh/resumed interactive
launches also pass `--dangerously-bypass-approvals-and-sandbox` to override
saved session restrictions. Standalone Claude Code
launches with `--dangerously-skip-permissions`. Its launcher also supplies
`skipDangerousModePermissionPrompt` to suppress the separate initial bypass
warning. Gas Town requires that same bypass contract for every role; the
launcher restores it even when a daemon or handoff reconstructs arguments.
Subagents inherit their principal's permission mode.

The orchestrator owns this default and invokes an agent-specific translator.
There is no public permission selector in this initial implementation. Future
restricted modes may be supported by compatible standalone adapters; Gas Town
must retain bypass for its automated execution contract. Automatic Review is
not an unattended execution guarantee.

Permission defaults do not authenticate clients or complete first-run
onboarding. Tests still verify those independent prerequisites. Live scenarios
exercise actual file edits, command/build execution and HTTPS access, including
delegated actions, without supplying their own permission overrides.

The Docker/Lima guest is the isolation boundary. `/lab/work` stays in the guest;
`/lab/infra`, `/lab/agents` and the selected orchestrator are read-only host
mounts. `/lab/git` and `/lab/recordings` are writable host directories. Neither
driver mounts the entire host home. Docker does not mount the Docker socket or
request privileged mode. Agents can use passwordless sudo inside the guest.
Network access and provisioned credentials can reach external services, and
agents can modify the writable host artifact directories. Full Access does
not remove those capabilities or isolate their external effects.
