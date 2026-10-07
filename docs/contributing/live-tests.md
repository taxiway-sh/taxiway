# Agent-driven live validation

Live validation lets an agent verify the feature it is working on in a real
Docker or Lima Taxiway lab, selected according to the implemented feature, as
a person would manually. Choose bounded actions and observable
results for that task; keep temporary scripts, logs and sanitized reports outside
Git. `tests/live/` provides reusable tooling, not a feature regression suite.
Orchestrator lifecycle nonregression assertions enrich the nine existing scenarios and
helpers in `internal/cli/orchestrator_e2e_test.go`; do not add parallel suites,
files, entry points, jobs or Makefile runners. See
[Testing](testing.md). Real model calls are explicitly opt-in and consume the
account's subscription allowance. Account-free E2Es do not prove real-account
model access, inference or delegation.

The shared helpers are in `tests/live/taxiway_live.py`. They use Python's
standard library and are usable for any feature.
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

If auth expires during validation, the failing action is not replayed automatically.
The next propagation runs another bounded preflight; after confirmed rejection,
reconnect once and explicitly rerun the failed scenario. There are no infinite
refresh/login/setup retries, and temporary-lab cleanup preserves the reference.
Native refresh success is established by a successful real request, never by
credential-file presence.
The helper does not synchronize subsequent refreshes between running labs.

Codex works differently: Taxiway prepares ChatGPT authentication in the
gateway from the existing host Codex login. Run `taxiway credentials codex`
once in each dev/e2e context before creating a Codex lab: `taxiway up` does not
prepare it, and without it the lab's LiteLLM gateway waits for a device login
until the `gateway` phase times out (tracked in #6). Do not copy Claude
credentials or host Codex auth files into Codex clients. API-key mode also uses
gateway credentials and is not handled by the Claude OAuth-copy helper.

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
direnv exec . ./taxiway credentials codex
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
delegation, or a particular model. Inspect actual effects and traces using the
[model gateway validation examples](model-gateway-tests.md).

## Lima driver and recording recipes

The nine automated E2E scenarios use Docker. Lima validation is a task-specific
manual check by an agent, selected here through the existing
[workflow skill](https://github.com/taxiway-sh/taxiway/blob/main/.agents/skills/taxiway-workflow/SKILL.md).
These recipes preserve the useful observations of the former native scripts;
they are not a committed executable suite. Docker results do not qualify Lima.
Choose only the observations relevant to your feature, and report which actually
ran. Mount, recording, containment and owned-cleanup observations also apply to
Docker: choose `driver="docker"` when that is the affected backend. Boot markers
and VM status/retry below are Lima-specific. Real-agent/gateway checks are
separate when the change requires them.

Use Python 3.11+, Lima and enough capacity for one 4-CPU/8-GiB VM. The first
creation downloads Ubuntu and installs guest dependencies. Build the worktree
CLI and load its isolated environment as shown above. Create a temporary script
outside Git and import the shared helpers with `PYTHONPATH="$PWD/tests/live"`.
Start with an owned, provider-free lab:

```python
from pathlib import Path
import os
from taxiway_live import command, guest, runtime_id, temporary_lab

with temporary_lab("claude-code", driver="lima", prepare_only=True,
                   timeout=900) as lab:
    runtime = runtime_id(lab)
    assert guest(lab, "test -s /run/lima-boot-done && test -w /lab/work && printf READY",
                 timeout=30) == b"READY"
    # Add only the observations needed for this feature here.
```

`prepare_only=True` provisions the guest but starts no agent roles or gateway.
It accesses no reference credentials and makes no model calls. This cannot
establish inference, gateway routing, permissions of an actual agent or delegation.
Provisioning is bounded (900 seconds in this example); native boot-start bounds
can also use `TAXIWAY_LIMA_START_TIMEOUT`. Keep guest checks at 10–30 seconds and
stop/start/removal bounded separately. The helper attempts owned removal even
if setup or an observation fails. A killed process still requires scoped inventory.

### Readiness, retry and restart

Fresh readiness means successful guest execution, writable `/lab/work` and a
nonempty `/run/lima-boot-done`, not merely a VM reported Running. When retry is
relevant, temporarily remove the owned lab's `phases/create.done` marker and
rename `/run/lima-boot-done` to `/run/lima-boot-done.taxiway-check` using guest
sudo. Keep the VM Running; a bounded `taxiway create <lab> --driver lima --type
claude-code` must fail with `Lima boot scripts have not finished`, leave the
create marker absent and preserve the existing VM. Capture the CLI output
locally for that specific assertion; do not print raw output. Restore the boot
file in `finally`, even after a failed assertion.

Use `subprocess.run(..., capture_output=True, timeout=30)` for that expected
rejection so you can inspect its exit code and specific stderr locally; the
shared `command` deliberately withholds error output.

Rerun create after restoring readiness: it must succeed and write `create.done`
while reusing the same runtime ID. For restart, run `taxiway down <lab>` (120-second
bound), verify the owned VM is Stopped, then `taxiway create <lab> --driver lima
--type claude-code` within the setup bound. Require guest execution and the boot
marker again. Do not apply boot-marker mutations to a reference or unrelated VM.

### Mounts and actual recording

Verify `/lab/infra/commands/bootstrap.sh` is readable, `/lab/work` exists and
`python3`, `tmux`, `asciinema`, `timeout` are installed. Write a harmless fixture
under `/lab/recordings` in the guest and read the same bytes from
`$TAXIWAY_LAB_STATE_DIR/<lab>/recordings` on the host, then remove that fixture.
This establishes writable mount and host visibility rather than guest-only state.

Create an owned tmux session named `claude-code` running a plain shell, set its
prefix to `C-a`, then use the real CLI:

```python
guest(lab, "tmux new-session -d -s claude-code 'bash --noprofile --norc'; "
           "tmux set-option -g prefix C-a", timeout=15)
command(["./taxiway", "record", "start", lab, "--name", "manual-check"], timeout=60)
```

Read the latest entry in the owned lab's `recordings.json`. Before sending proof
output, poll for at most 30 seconds until its `recorder_session` has a nonempty
`@taxiway-recorder-client` tty present in `tmux list-clients`. Send a unique harmless
`printf` marker to the target session and inspect the host asciicast: require a
version-2 header and the marker in actual output events (not just a file existing).
Ignore only the writer's current partial JSON line while polling.

Run `record stop <lab>` (60-second bound). Require index state `stopped`, the
recorder tmux session absent, the target `claude-code` session still present and
the captured marker preserved. Run `record rm <lab> --id <id>` and verify its
cast disappears. Keep casts and local diagnostic output outside Git.

### Recording containment and stopped-VM recovery

If host-artifact containment is affected, save the owned recording index bytes
and create a harmless outside `sentinel.cast` in a host temporary directory.
Using the writable guest mount, poison its latest index entry first with an
outside `cast_path_host`, then with a symlink under the recordings directory
pointing outside. For each fixture, `record list`, `record rm --id <id>` and
`record analyze --prompt-only` must fail. Verify the sentinel bytes and poisoned
index are unchanged by rejected commands. Restore the original index and remove
the fixture symlink in `finally`. A subsequent valid `analyze --prompt-only`
must include the owned captured artifact. Never use a real private file as sentinel.

For stopped-VM recovery, start another recording and wait for a nonempty host
cast. Stop the VM with `down`, then `record stop` (30-second bound): its index
must become stopped while the cast remains. `record rm --id <id>` must work
while the VM is stopped and remove that cast. This checks offline reconciliation,
not real-agent session recovery.

### Failure, timeout and cleanup evidence

When checking the live helper's resource ownership, select an intentional
`guest(lab, "exit 7", timeout=10)` or `guest(lab, "exec sleep 30", timeout=1)`
inside `temporary_lab`. Catch only the expected exit-7 RuntimeError or host
`subprocess.TimeoutExpired`; setup/cleanup failures are not expected success.
After the context exits, verify the owned lab state and runtime VM are absent.
Compare bounded `limactl list --format '{{.Name}} {{.Status}}'` inventories before
and after: unrelated instances must still exist with their original statuses.
These observations validate helper cleanup, not a new product E2E scenario.

Report the commit (including tracked changes), selected checks, actual effects,
cleanup evidence and limitations. Missing prerequisites, timeout during setup,
failed cleanup or an unexecuted observation must be reported, not counted as a
pass. Preserve partially created resources until targeted cleanup can succeed;
never globally stop Lima VMs or prune Docker.

## Validate a feature

Create a temporary script outside the checkout (for example under `/tmp`) and
import the helpers by running from the worktree:

```bash
PYTHONPATH="$PWD/tests/live" direnv exec . python3 /tmp/taxiway-feature-check.py
```

Do not commit the temporary script.
Select assertions for the feature rather than rerunning a fixed account suite.

Available helpers:

| Helper | Purpose |
|---|---|
| `validate_context()` | Require an isolated dev/e2e environment and scoped runtime paths |
| `guest(lab, script, ...)` | Execute a guest shell script through the lab's persisted Docker/Lima driver; return captured bytes |
| `run_in_lab(lab, argv, agent=..., ...)` | Execute argv with the lab's gateway environment and a guest timeout |
| `propagate_claude_auth(source, target, overwrite=False)` | Reuse a reference OAuth login without copying gateway secrets |
| `temporary_lab(orch, auth_lab=..., ...)` | Create an owned lab and remove it on normal exit or an exception |
| `command(argv, ...)` | Run a bounded host command; capture output without printing it |
| `runtime_id(lab)` | Resolve the driver identifier from the active context |

For example, this temporary check verifies filesystem persistence across an
orchestrator session restart. Credential presence here does not establish that
authentication works; add a bounded real request if that is the feature:

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
    print("PASS filesystem persistence across restart")
```

Add the assertions that establish your feature's behavior: inspect a resulting
file for an edit, client tool events for tool execution, child session metadata
for delegation, and gateway/Langfuse generations for actual model calls.
Reuse existing E2Es for durable regression assertions; real-account observations
remain task-specific live evidence.

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
onboarding. Tests still verify those independent prerequisites. For affected permission behavior, choose actual file edits, command/build
execution and HTTPS access, including delegated actions, without supplying
extra permission overrides that conceal the shipped defaults.

The orchestrator E2E scenarios also send a unique harmless prompt through the
real interactive CLI in its actual tmux session. Their controlled upstream
computes a digest absent from the prompt; terminal echo, a running process and
gateway health cannot satisfy this check. A rejected Codex request must show
the controlled error without a completed digest, and a subsequent normal
request must complete. Gas Town checks its persistent roles and a newly created
crew workspace, including that workspace's trust settings.

Claude-based E2E sessions select API-key mode with their controlled gateway and
use `tests/fixtures/claude-onboarding.json`, containing
only the two public first-run fields verified after a person completed native
Claude Code `2.1.289` onboarding. The existing live helper merges those fields
without copying accounts, OAuth credentials, preferences, trust or gateway
keys. Prepare/run and phase-by-phase apply this fixture before starting the
agent. The Claude `up` scenario first verifies that missing setup blocks
interactive readiness, then applies the fixture and explicitly restarts the
session. Successful response checks cover completed native setup; they do not
claim that a user's first-ever interactive launch needs no onboarding.

The Docker/Lima guest is the isolation boundary. `/lab/work` stays in the guest;
`/lab/infra`, `/lab/agents` and the selected orchestrator are read-only host
mounts. `/lab/git` and `/lab/recordings` are writable host directories. Neither
driver mounts the entire host home. Docker does not mount the Docker socket or
request privileged mode. Agents can use passwordless sudo inside the guest.
Network access and provisioned credentials can reach external services, and
agents can modify the writable host artifact directories. Full Access does
not remove those capabilities or isolate their external effects.
