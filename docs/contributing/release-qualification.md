# Release qualification recipe

Use this recipe before proposing a Taxiway release. It qualifies Taxiway, not
an evaluation runner, scientific protocol or model quality. The canonical
`taxiway-release` skill is shared by Codex and Claude Code. Invoke
`$taxiway-release` in Codex or `/taxiway-release` in Claude Code, or ask the agent
to prepare a release. Qualification does not authorize a tag, push or publication.
See [Release](release.md) for the separately authorized publication procedure.

## Candidate, prerequisites and evidence

Start from the exact clean committed candidate in a dedicated worktree. Read
[Testing](testing.md), [live test instructions](https://github.com/taxiway-sh/taxiway/blob/main/tests/live/AGENTS.md) and
[Live validation](live-tests.md). Required tools depend on the checks: Go, Bash,
Python 3, Node/npm, GoReleaser, Docker, Lima and GitHub CLI. Missing prerequisites
are `NOT_EXECUTED`, never a pass. Use the supported tool versions documented by
the repository. Do not update provider clients or the catalog during a recipe.

Reconcile existing test processes, context IDs, reference labs and prior proof
before starting. Coordinate one heavy Docker/Lima run at a time. Fast local
checks generally take minutes; all orchestrator E2Es, snapshots and installation
matrices can take tens of minutes each. Live calls consume account usage and
need explicit scope, bounded requests and usable authentication. Timeouts are
failure evidence, not permission to duplicate an uncertain in-flight run.

The lightweight helper records evidence; it does not schedule a campaign,
publish, discover all failures, sanitize logs or certify a recipe automatically.
Its JSON/Markdown reports stay outside Git. Use a private directory; do not
pass secrets in commands, notes or filenames, dump environment variables, or
export raw credential files, account journals or model transcripts. Log files
are private (`0600` under a `0700` directory); inspect and sanitize before sharing.

```bash
candidate=$(git rev-parse HEAD)
recipe_dir=$(mktemp -d /tmp/taxiway-qualification-parent.XXXXXX)/pre
python3 tests/release/qualify.py --report "$recipe_dir" init --candidate "$candidate" --driver docker
python3 tests/release/qualify.py --report "$recipe_dir" run core --timeout 900 -- make test-unit
python3 tests/release/qualify.py --report "$recipe_dir" run scripts --timeout 900 -- make test-scripts
python3 tests/release/qualify.py --report "$recipe_dir" report
```

`report` exits nonzero while any check is FAIL or NOT_EXECUTED. `run` invokes
argv without a shell, records the command/exit and a unique proof file, and kills
the command's process group on timeout or interruption. Cleanup of Docker/Lima
resources still requires the owning scenario. Known skip markers yield
NOT_EXECUTED. A zero exit does **not** prove that tests actually ran: inspect test
counts, skips, assertions and coverage before accepting PASS. Never pipe a check
through a filter that hides its failure. Pass shell scripts as `bash script.sh`.

Each proof must identify candidate SHA/tag, platform/architecture and driver,
binary/runtime paths, requested and observed versions/models, command, outcome,
coverage limits and pending actions. Record those observations in sanitized
proof files for manual or external checks. The helper collects platform,
checkout, driver and command automatically; client/runtime observations require
inspection. For example, after verifying all GitHub E2E jobs and their head SHA:

```bash
python3 tests/release/qualify.py --report "$recipe_dir" record e2e-codex \
  --candidate "$candidate" --status PASS --evidence /tmp/codex-e2e-proof.txt \
  --note 'All three Codex scenarios passed on this SHA; simulated provider, Docker/amd64.'
```

`record` requires an exact matching candidate and an existing proof for PASS/FAIL.
It records an inspected assertion, not automatic validation of a GitHub run or
document. Use NOT_EXECUTED and explain the missing prerequisite/pending action
when no actual proof exists. Keep previous logs; a retry creates a new log.
Do not initialize over an existing report. On resume inspect real processes and
owned resources before retrying; interrupted results stay NOT_EXECUTED. Reports
cannot be resumed against another HEAD or dirty source. Changes require a new
clean candidate and new report; old proofs are context, not current validation.
No monitoring continues after the agent session ends.

## Before publication

The table is the baseline recipe. Record each row separately using its check ID.
If a supported combination cannot be tested, leave NOT_EXECUTED and describe
the coverage limitation to the user; do not silently narrow the release promise.

The helper lists the full baseline, not a new requirement to spend allowance on
every adapter for every scoped release. A deliberately restricted qualification
(for example Claude/Docker only) leaves other rows NOT_EXECUTED and presents
that restriction explicitly. Its nonzero report prevents a claim of complete
baseline coverage; the user can decide whether that scope meets the release
promise. Do not turn untested supported paths into PASS or hide them.

| Check ID | Existing checks / actions | Acceptance and proof |
|---|---|---|
| `core` | `make lint build test-unit`; helper tests: `make test-release-tools` | Build/lint/unit results, counts, actual candidate; no unexpected skips. |
| `scripts` | `make test-scripts` | Installer, settings, versions, previews, confirmations and runtime contracts pass. No dry-run mutation. |
| `site` | In `site/`: `npm ci`, `npm test`, `npm run build` | Routes/navigation/build pass; manually inspect changed pages, links and examples. |
| `gateway` | Gateway-routed assertions in the existing orchestrator lifecycle targets (`make test-e2e-claude-code`, `make test-e2e-codex`, `make test-e2e-gastown`) | Actual lab gateway routing, streaming, tool replay and provider errors survive configured/start/restart lifecycle stages using controlled providers; no real-account claim or separate protocol runner. |
| `e2e-codex` | `make test-e2e-codex` | Up, prepare/run and phase-by-phase all pass, including models, autonomous permissions, exact versions and restart. |
| `e2e-claude-code` | `make test-e2e-claude-code` | All three scenarios pass, model/config propagation, trust/onboarding and restart checked. |
| `e2e-gastown` | `make test-e2e-gastown` | All three scenarios pass, deacon/startup sessions and doctor health, renewal/handoff configuration checked. |
| `live-codex` | Task-specific bounded check with `taxiway_live.py`; see [model gateway validation](model-gateway-tests.md) | Real principal/child relationships, requested/observed models and gateway calls with output tokens. Verify relevant tool effects and actual interactive tmux completion. Keep scripts/results temporary. |
| `live-claude` | Task-specific bounded check with a verified Claude reference; see [model gateway validation](model-gateway-tests.md) | Real principal/delegation and auth reuse/restart. Verify relevant tool effects and interactive completion without onboarding. Keep scripts/results temporary. |
| `live-gastown` | Task-specific bounded Gas Town observations; see [model gateway validation](model-gateway-tests.md) | Actual Deacon patrol progress, relevant handoffs and principal/subagent requests. Attach to Mayor and verify bounded real work without auth/permission/onboarding loops; does not qualify a full refinery workload. |
| `lima` | Selected [Lima live recipes](live-tests.md#lima-driver-and-recording-recipes) using `taxiway_live.py` | Real guest readiness/retry, mounts, recording and stop/start observations; separately verify gateway/inference when in scope and state actual adapters. Current Docker E2Es do not qualify Lima. No separate native suite is endorsed. |
| `recording` | Recording assertions in the existing Go orchestrator scenarios; scoped temporary Lima checks when qualifying Lima | Actual cast output, recorder stop preserves agent, containment and interrupted/stopped guest recovery. Verify actual Lima behavior separately; also manually verify browser replay/export offline. |
| `docs-cli` | Compare docs/help to CLI, settings, manifests, installer and runtime scripts; execute representative documented examples | See checklist below; precise findings, corrections or documented limitations. Text matching alone is insufficient. |
| `package` | `goreleaser check`; `goreleaser release --snapshot --clean`; inspect archives/checksums/installer and run extracted host binary | Correct four platform archives, executable, matching runtime, catalog, local player/scripts/styles, metadata and paths. Snapshot is not a published-release installation. |
| `isolation-cleanup` | Scoped ownership inventory before/after; two contexts/labs; supported failure/retry cases | No collisions; intended mounts only; host agent config preserved; owned guests/gateways/networks/volumes removed; unrelated instances unchanged. |
| `beta-compat` | Review code/tests for historical migration and compatibility branches | Beta policy respected; distinguish unnecessary cross-version shims/tests from required current idempotency, recovery and reset behavior. Propose focused issues for existing discoveries, not blanket removal. |
| `breaking-changes` | Review behavior and diff since the last published release, not only commit labels | Release notes identify each breaking change with before/after, impact and user action (recreate lab, change setting, etc.). Breaking changes are allowed; no migration mechanism is implied. |
| `review` | Different agent reviews candidate diff, findings, fixes and actual proof | Independent review and correction evidence; no self-review presented as independent. |

Durable assertions enrich only the nine existing scenarios and helpers in
`internal/cli/orchestrator_e2e_test.go`; do not add parallel scenario files,
suites, entry points, jobs or Makefile runners. Run temporary live scripts through `direnv exec . python3` with
`PYTHONPATH="$PWD/tests/live"` from the worktree. Follow
[model gateway validation](model-gateway-tests.md) and the live guide for setup
and preflight; do not reinvent authentication plumbing.
Build the candidate binary before testing. For missing/rejected authentication,
give the user the exact native reconnection command for the owned reference lab,
then verify a bounded real request. Reuse `taxiway_live.py` for authorized labs.
Distinguish expired/rejected auth from network/model/gateway failure. Never infer
validity from credential file presence or copy Codex client auth into guests.
Continue independent checks while authentication is pending.

The existing `e2e.yml` workflow may replace local orchestrator E2Es when run on
the exact candidate. Verify all nine scenarios, job conclusions and head SHA,
including skips; save a bounded sanitized summary with run URL. Do not accept
core CI alone or a green previous commit. Dispatch/push only when authorized by
the contribution workflow. Do not add a separate job for each recipe assertion.

### Private security review

Before accepting `review`, consult Codex Security Cloud privately for the
repository. Record the scan date, scanned commit SHA and covered paths/checks
in private evidence tied to the candidate. Check freshness and coverage: a scan
of an older SHA does not qualify the candidate unless the intervening changes
have been explicitly assessed. Missing access, unavailable results, unknown
coverage or an obsolete scan leave this security review incomplete; they never
mean that no findings exist.

Triage each finding privately against the candidate, including severity,
relevance, remediation and verification. An unresolved serious vulnerability
that affects the release blocks publication. Preserve a private disposition
for other findings and explain any remaining uncertainty to the user. Do not
mark `review` PASS until both independent feature review and this security
review have sufficient evidence.

Never put vulnerability details, reproduction steps, exploit material, finding
screenshots or raw scanner output in public issues, pull requests, release notes
or public logs. Keep those details in the private security channel and private
proof; coordinate any necessary private remediation with the user. Public
communication contains only an aggregate status, such as security review
complete, incomplete or blocked, without disclosing findings. Opening Security
Cloud or receiving an empty/unavailable response is not evidence that findings
were consulted.

### Documentation and behavior checklist

- Installation from checkout and archives: paths, runtime resolution, checksums,
  explicit version, writable install directory and errors agree with code.
- CLI help, options, defaults and examples match actual behavior and exit codes;
  invalid settings/models fail clearly. No obsolete model IDs or auth advice.
- Versions and model aliases/provider routing match manifests/catalog and actual
  processes; pins and settings survive restart/handoff without silent updates.
- Auth docs explain gateway versus guest responsibility, onboarding, preflight
  and recovery. No unsupported claim of token refresh, isolation or revocation.
- Guest permissions are accurately described; real tool effects require no
  approval, host client configuration remains unchanged.
- Create/up/prepare/run/stop/restart/reset/rm/destroy descriptions match state,
  declined confirmations and no-op previews; recordings and data preservation
  rules are clear. Include unavailable gateway/download and interrupted retry.
- Langfuse receives real generations with models/session identifiers; check
  operation with and without observability. Explicitly state any error/retry or
  subagent trace gaps; a health endpoint alone proves no trace coverage.
- Recording captures the intended tmux target; document capture limits. Replay
  works with network disabled, without CDN assets. Safe stop/recovery is distinct
  from killing the agent.
- Driver readiness, supported architectures, writable mounts, isolation and
  cleanup scope match reality; no global prune. Cross-check linked pages and
  site rendering, not only Markdown existence.

Investigate acceptance-critical drift such as #50 against the current code;
report/fix through its appropriate issue and reviewed branch. Unrelated bugs or
dependency defects become sanitized issue proposals, not an unbounded release
refactor. Do not add historical compatibility tests for this beta project.

For `breaking-changes`, identify the previous published tag explicitly and review
the commit range, configuration/CLI/runtime contracts and changed defaults.
Record both the resolved base tag SHA and final candidate SHA in the proof so
the reviewed range remains unambiguous if a remote tag changes.
Conventional Commit `!` or `BREAKING CHANGE:` markers help surface changes, but
their absence does not prove compatibility. Prepare concrete release-note text
for user-visible breaks, including required user action and limits. Check that
the GoReleaser-generated notes actually convey those changes; supplement the
proposed notes when necessary. Do not add migration code merely to avoid a
breaking release. `beta-compat` should flag historical baggage for focused fixes
while preserving current-state idempotency/recovery tests.
The current GoReleaser message grouping does not produce a dedicated breaking
changes section or render every commit footer. Prepare an explicit reviewed
section for the publication notes; `feat!` alone does not ensure adequate advice.

## After authorized publication

Create a separate report with `init --phase post --candidate vX.Y.Z` from a clean
checkout of that exact tag. Explicit tags are required; `latest` cannot identify
stable evidence. Retain the pre-publication report and link the release/tag SHA.

| Check ID | Action | Acceptance |
|---|---|---|
| `published-assets` | Inspect the exact GitHub release, notes, archive names, checksums and installer | All supported assets present; checksums verify downloaded files; binary/assets/tag agree. |
| `install-matrix` | Inspect automatic `install.yml` for that exact release input, or authorized rerun | All six x64 platform × driver jobs pass; inspect each job, skips and logs. Workflow head alone is not proof of installed tag. |
| `mac-install` | Install exact published archive in isolated paths on the user's actual Mac | Apple Silicon/Docker Desktop installation, version, runtime/catalog/player and CLI smoke pass. Preserve existing host installation. |
| `installed-smoke` | Use installed binary/runtime to create, run, stop/restart and delete a scoped lab; representative bounded real-agent/recording smoke | Installed artifacts actually work; report adapter/driver/account coverage distinctly from creation-only matrix. |

See [Installation qualification](installation-qualification.md) for matrix
limits. A snapshot, source-tree live success or creation-only install smoke is
not authenticated validation of all published binaries. Qualification failure
after publication does not retract the release automatically: report it and
propose a focused fix, do not silently overwrite a tag or republish assets.

## Finish and cleanup

Inspect the report, proof and remaining FAIL/NOT_EXECUTED rows with an independent
reviewer. Present qualified combinations, unsupported/unverified claims, pending
manual actions and release blockers. All PASS is an evidence summary, not a
license to publish: obtain the separate explicit publication authorization.

Scenario-owned labs are cleaned by existing helpers. Preserve reference/manual
labs until authorized cleanup. If this qualification owns the whole worktree
instance, use its scoped destroy procedure only after confirming ownership and
recording/export preservation. Verify remaining Docker containers/networks/
volumes and Lima guests by context, including the proxy; never global prune or
destroy another checkout. Keep private proof outside Git and follow
[post-merge cleanup](development.md#post-merge-cleanup) for merged work.
