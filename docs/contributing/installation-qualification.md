# Installation qualification

This page describes how to verify a published Taxiway release on macOS, Linux,
and Windows WSL2 using the `Install integration` GitHub Actions workflow
(`.github/workflows/install.yml`). For source-code test suites, see
[Testing](testing.md). For publication, see [Release](release.md).

## Running the workflow

In GitHub Actions, select **Install integration**, then **Run workflow**.
Select `main` and a release tag, or `latest`.

From the command line:

```bash
gh workflow run install.yml --repo taxiway-sh/taxiway \
  --ref main -f release=latest
```

Use an explicit release tag to qualify a specific version. Use `latest` for
a check of the most recent release.

The workflow installs the published release selected by the `release` input,
including its binary and runtime assets. It does not build Taxiway from source.

The workflow also runs on pushes changing `install.yml`. It has no scheduled
trigger and does not run on every pull request. Its driver matrices use
`fail-fast: false`, so a failed job does not cancel the other combinations.

## Scenarios

Each platform × driver combination gets its own runner job. Every job uses
the same five steps:

| Step | Verification |
|---|---|
| Install prerequisites | Docker and Compose for both drivers; Lima and its VM dependencies for Lima labs |
| Install Taxiway | Published installer, binary version, runtime assets, and `taxiway init` |
| Verify installation | Shared services ready; create a Codex lab; check Taxiway state and the actual container or VM |
| Collect failure diagnostics | Print runtime and platform diagnostics if a preceding step fails |
| Clean up | Remove the lab and verify its state and driver resource are gone; clean up shared services |

The readiness check uses `taxiway status` to verify Docker availability and
the proxy and Langfuse stack. After `create`, the expected lab health is
`degraded`, its phase is `created`, and its driver matches the job. The
underlying Docker container must be `running`, or the Lima instance `Running`.

This validates installation and lab creation before orchestrator provisioning.
It does not run authenticated agents or measure Gas Town performance.

## Installation runners

Each row below runs twice: once for Docker and once for Lima.

| Platform | Runner | Dependencies |
|---|---|---|
| Linux x64 | `ubuntu-24.04` | Runner Docker; QEMU/KVM and Lima for Lima labs |
| macOS Intel | `macos-15-intel` | Homebrew Docker, Compose, Colima, and Lima |
| Windows WSL2 x64 | `windows-2025` | Ubuntu 24.04 with Docker/Compose; QEMU and Lima for Lima labs |

On macOS, Colima provides Docker for the shared services in both jobs. Lima
creates a separate VM for a Lima lab. This workflow does not test Docker
Desktop. Linux and Windows use the `LIMA_VERSION` declared in the workflow;
macOS uses the Homebrew package.

On Windows, Taxiway and all dependencies run inside Ubuntu WSL2. The workflow
allocates 12 GB and four processors to WSL and enables nested virtualization;
these are test settings, not measured user requirements. It probes KVM when
`/dev/kvm` exists and permits QEMU software emulation (TCG) otherwise. A passing
job does not by itself prove hardware acceleration or workload performance.

## Qualification and limitations

Published binaries cover macOS and Linux on x64 and ARM64. The installation
matrix covers x64 runners only. A successful run qualifies the selected
release on those runner configurations. It does not establish compatibility
with every machine that can run the published binaries.

- Windows Server 2025 exercises the WSL2 installation path, but does not
  directly certify Windows 11.
- macOS Apple Silicon, Linux ARM64, and Windows ARM64 are outside the runner
  matrix.
- Creating a lab does not qualify authenticated orchestrator execution or
  workload performance.

Qualification requires all six platform × driver jobs to succeed, including
their cleanup checks. A failed, cancelled, or skipped job leaves that
combination unqualified. Inspect each result before reporting release coverage.

## Failure diagnostics

For installation failures, open the failing step and **Collect failure
diagnostics** in the Actions logs. The latter records available Taxiway,
Docker, Lima, disk, and platform information before cleanup. There is no
separate diagnostic artifact. A cleanup-only failure occurs after collection,
so inspect the cleanup step's own output in that case.

## Updating qualification

When changing `install.yml`, preserve the separate driver jobs and common
five-step scenario. Keep prerequisites consistent with
[Installation](../reference/installation.md) and update this page when the
scenario or runner coverage changes.

Keep version-specific results and run links in the release notes or the
relevant issue. This page describes the qualification procedure and its scope.
