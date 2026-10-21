# Cloud session environment

Pitfalls of running Taxiway checks in a Claude Code cloud container. Two E2E
runs of [#185](https://github.com/taxiway-sh/taxiway/pull/185) were invalid
before these were known. Each section says whether it was verified in a cloud
session or comes from that report.

## Docker daemon

The daemon is installed but not running when the session starts. Start it
detached from an ordinary shell command, not as a bounded background task:

```bash
nohup dockerd > /tmp/dockerd.log 2>&1 < /dev/null &
docker info --format '{{.ServerVersion}}'
```

A background task is killed at its 2-hour limit, and `dockerd` with it, taking
every lab and the shared services down mid-run. The aggregate E2E targets and
the Gas City scenarios can exceed that limit. A run interrupted this way has no
verdict.

- Verified: the detached start, and the daemon staying up across later commands.
- From #185: the kill of a background-task `dockerd` at the 2-hour limit.

## Root user

The container runs as `root`. Tests that rely on a permission failure pass the
operation instead of failing it, for example
`TestRecordRmSaveFailurePreservesCast`, and recording indexing behaves
differently. Run Go tests and E2E as an unprivileged `e2e` user on a copy of the
worktree it owns:

```bash
id e2e >/dev/null 2>&1 || useradd -m e2e
usermod -aG docker e2e
cp -r . /home/e2e/taxiway && chown -R e2e: /home/e2e/taxiway
su e2e -c 'cd /home/e2e/taxiway && PATH=/usr/local/go/bin:$PATH make test-unit'
```

Use `su e2e -c` rather than `su - e2e` so the proxy environment variables are
kept. Commit from the original worktree, not from the copy.

- Verified: `TestRecordRmSaveFailurePreservesCast` fails as `root` and passes as
  `e2e` on the same commit; module downloads work for `e2e` through `su -c`.
- Not verified here: a full E2E run as `e2e`.

## Network proxy and images

Outbound HTTPS goes through an agent proxy that re-terminates TLS. Host tools
are preconfigured; the session's proxy README (`/root/.ccr/README.md`) names the
CA bundle, `/root/.ccr/ca-bundle.crt`. Container traffic is intercepted too:
an HTTPS request from a container that does not trust that CA is reset, and the
same request succeeds once the bundle is the container's trust store. Lab guests
start from `ubuntu:24.04` and download over HTTPS during provisioning, so they
need the CA as well.

- Workaround, never committed: build a local image from `ubuntu:24.04` that adds
  the CA to its trust store and tag it `ubuntu:24.04` locally (the Docker driver
  does not pull an image already present). Report it with the E2E result, as
  [`steward`](../../steward/SKILL.md#ci) requires for local workarounds.
- Docker Hub: if pulls hit rate limits or policy denials, set
  `"registry-mirrors"` in `/etc/docker/daemon.json` and restart `dockerd`.

- Verified: the README and bundle path; container HTTPS reset without the CA and
  success with it; Docker Hub pulls (`hello-world`, `alpine`) worked without a
  mirror in that session.
- From #185: the image and mirror workarounds needed for the twelve scenarios.

## GitHub API

`gh` GraphQL is refused (HTTP 403), so commands built on it, such as
`gh pr view`, `gh pr list` and `gh issue view`, fail. Use REST through
`gh api repos/taxiway-sh/taxiway/...`, and the GitHub MCP tools for PR creation
and reviews.

- Verified: `gh api graphql` and `gh pr view` refused;
  `gh api repos/taxiway-sh/taxiway/issues/<n>` works.

## Out of reach

There is no `/dev/kvm` and no Lima, and no maintainer account: Lima, macOS host
and real-account checks cannot run here (verified: no `/dev/kvm`, no `limactl`).
See [Live validation](../SKILL.md#live-validation-docker-or-lima).
