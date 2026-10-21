# Cloud session environment

Pitfalls of running Taxiway checks in a Claude Code cloud container. Two E2E
runs of [#185](https://github.com/taxiway-sh/taxiway/pull/185) were invalid
before these were known. Each claim is labeled **verified** (checked in a cloud
session) or **#185** (from that report, not reproduced since).

## Docker daemon

- The daemon is installed but not running when the session starts (**verified**).
  Start it detached from an ordinary shell command, not as a bounded background
  task; it then stays up across later commands (**verified**):

  ```bash
  nohup dockerd > /tmp/dockerd.log 2>&1 < /dev/null &
  docker info --format '{{.ServerVersion}}'
  ```

- A background task is killed at its 2-hour limit, and `dockerd` with it, taking
  every lab and the shared services down mid-run (**#185**). The
  twelve-scenario targets `make test-e2e` and `make test-e2e-only` take about
  2.5 hours
  ([Testing](../../../../docs/contributing/testing.md#end-to-end)), so they
  cannot finish under that limit. A run interrupted this way has no verdict.

## Root user

- The container runs as `root` (**verified**). Tests that rely on a permission
  failure pass the operation instead of failing it:
  `TestRecordRmSaveFailurePreservesCast` fails as `root` and passes as an
  unprivileged user on the same commit (**verified**). Recording indexing is
  affected the same way (**#185**).
- Run Go tests and E2E as an unprivileged `e2e` user on a copy of the worktree it
  owns, and commit from the original worktree. `su e2e -c` (not `su - e2e`)
  keeps the proxy environment variables, so module downloads work (**verified**
  for `go test`; a full E2E run as `e2e` is **#185**):

  ```bash
  id e2e >/dev/null 2>&1 || useradd -m e2e
  usermod -aG docker e2e
  cp -r . /home/e2e/taxiway && chown -R e2e: /home/e2e/taxiway
  su e2e -c 'cd /home/e2e/taxiway && PATH=/usr/local/go/bin:$PATH make test-unit'
  ```

## Network proxy and images

- Outbound HTTPS goes through an agent proxy that re-terminates TLS. Host tools
  are preconfigured: `SSL_CERT_FILE` and `REQUESTS_CA_BUNDLE` name the CA bundle,
  and the session's proxy README documents per-tool fixes (**verified**).
- Container traffic is intercepted too: an HTTPS request from a container that
  does not trust that CA is reset, and succeeds once the bundle is the
  container's trust store (**verified**). Lab guests start from `ubuntu:24.04`
  and download over HTTPS during provisioning, so they need the CA as well.
- Workaround, never committed: build a local image from `ubuntu:24.04` that adds
  the CA (`$SSL_CERT_FILE`) to its trust store and tag it `ubuntu:24.04` locally;
  the Docker driver does not pull an image already present (**#185** for the
  workaround; the driver behavior is in `internal/driver/docker.go`). Report it
  with the E2E result, as [`steward`](../../steward/SKILL.md#ci) requires for
  local workarounds.
- Docker Hub pulls (`hello-world`, `alpine`) worked without a mirror
  (**verified**). If pulls hit rate limits or policy denials, set
  `"registry-mirrors"` in `/etc/docker/daemon.json` and restart `dockerd`
  (**#185**).

## GitHub API

- `gh` GraphQL is refused with HTTP 403, so `gh api graphql` and commands built
  on GraphQL such as `gh pr view` fail (**verified**); REST through
  `gh api repos/taxiway-sh/taxiway/...` works (**verified**). Use REST, and the
  GitHub MCP tools for PR creation and reviews.

## Out of reach

- There is no `/dev/kvm` and no `limactl` (**verified**), and no maintainer
  account: Lima, macOS host and real-account checks cannot run here. See
  [Live validation](../SKILL.md#live-validation-docker-or-lima).
