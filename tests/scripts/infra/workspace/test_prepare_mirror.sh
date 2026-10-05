#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCRIPT="$SCRIPT_DIR/../../../../infra/workspace/prepare-mirror.sh"
source "$SCRIPT"
task_tmp="$(mktemp -d)"
trap 'rm -rf "$task_tmp"' EXIT
export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_SYSTEM=/dev/null
source_repo="$task_tmp/source"
host_mirror="$task_tmp/host.git"
guest_mirror="$task_tmp/guest.git"
checkout="$task_tmp/checkout"
git init -q "$source_repo"
git -C "$source_repo" -c user.name=Fixture -c user.email=fixture@example.invalid \
    commit -q --allow-empty -m initial
git clone -q --mirror "$source_repo" "$host_mirror"
workspace_prepare_mirror "$host_mirror" "$guest_mirror" >/dev/null 2>&1
[[ "$(git -C "$host_mirror" rev-parse HEAD)" == "$(git -C "$guest_mirror" rev-parse HEAD)" ]]
[[ ! -e "$guest_mirror/objects/info/alternates" ]]
for object in "$host_mirror"/objects/??/*; do
    [[ -f "$object" ]] || continue
    [[ ! "$object" -ef "$guest_mirror/objects/${object#"$host_mirror/objects/"}" ]]
done
echo 'PASS guest mirror uses independent objects'

git clone -q "$guest_mirror" "$checkout"
git -C "$checkout" -c user.name=Fixture -c user.email=fixture@example.invalid \
    commit -q --allow-empty -m guest
git -C "$checkout" push -q origin HEAD:refs/heads/guest-work
guest_head="$(git -C "$guest_mirror" rev-parse refs/heads/guest-work)"
if git -C "$host_mirror" show-ref --verify --quiet refs/heads/guest-work; then
    echo 'FAIL guest push reached host mirror' >&2
    exit 1
fi
echo 'PASS guest pushes stay in guest storage'

git -C "$source_repo" -c user.name=Fixture -c user.email=fixture@example.invalid \
    commit -q --allow-empty -m advance
# Guest hooks and executable settings never become host mirror metadata.
marker="$task_tmp/guest-metadata-executed"
printf '#!/bin/sh\nprintf harmless > %q\n' "$marker" > "$guest_mirror/hooks/reference-transaction"
chmod +x "$guest_mirror/hooks/reference-transaction"
printf '#!/bin/sh\nprintf harmless > %q\nexit 1\n' "$marker" > "$task_tmp/guest-ssh"
chmod +x "$task_tmp/guest-ssh"
git -C "$guest_mirror" config core.sshCommand "$task_tmp/guest-ssh"
git -C "$host_mirror" remote update --prune >/dev/null 2>&1
[[ ! -e "$marker" ]]
[[ ! -e "$host_mirror/hooks/reference-transaction" ]]
if git -C "$host_mirror" config --get core.sshCommand >/dev/null; then
    echo 'FAIL guest configuration reached host mirror' >&2
    exit 1
fi
rm "$guest_mirror/hooks/reference-transaction"
git -C "$guest_mirror" config --unset core.sshCommand
echo 'PASS host refresh never consumes guest hooks or executable configuration'
workspace_prepare_mirror "$host_mirror" "$guest_mirror" >/dev/null 2>&1
[[ "$(git -C "$guest_mirror" rev-parse refs/heads/guest-work)" == "$guest_head" ]]
branch="$(git -C "$source_repo" symbolic-ref HEAD)"
[[ "$(git -C "$guest_mirror" rev-parse "$branch")" == "$(git -C "$source_repo" rev-parse HEAD)" ]]
workspace_prepare_mirror "$host_mirror" "$guest_mirror" >/dev/null 2>&1
echo 'PASS refresh is idempotent and preserves guest-only refs'

for invalid in file:///tmp/repo.git file:///lab/git/nested/repo.git https://example.invalid/repo; do
    if TAXIWAY_REPO_FORK_URL="$invalid" bash "$SCRIPT" >/dev/null 2>&1; then
        echo 'FAIL invalid mirror accepted' >&2
        exit 1
    fi
done
echo 'PASS unmanaged mirror paths rejected'
