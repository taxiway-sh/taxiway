#!/usr/bin/env bash
# Mark a Taxiway-managed workspace as trusted by Claude Code.

set -euo pipefail

workspace_path="${TAXIWAY_WORKSPACE_TRUST_PATH:-}"
if (( $# > 0 )); then
    echo "trust-workspace.sh accepts no arguments; set TAXIWAY_WORKSPACE_TRUST_PATH" >&2
    exit 1
fi
if [[ -z "$workspace_path" || "$workspace_path" != /* ]]; then
    echo "TAXIWAY_WORKSPACE_TRUST_PATH must be an absolute path" >&2
    exit 1
fi
case "$workspace_path" in
    *$'\n'*|*$'\r'*)
        echo "TAXIWAY_WORKSPACE_TRUST_PATH must not contain newlines" >&2
        exit 1
        ;;
esac

python3 - "$HOME/.claude.json" "$workspace_path" <<'PY'
import fcntl
import json
import os
from pathlib import Path
import sys
import tempfile

config = Path(sys.argv[1])
workspace = sys.argv[2]
config.parent.mkdir(parents=True, exist_ok=True)
# A separate inode keeps the lock stable across atomic replacements. This
# serializes Taxiway hooks, not writes made by Claude Code itself.
with os.fdopen(os.open(str(config) + ".taxiway.lock", os.O_CREAT | os.O_RDWR, 0o600), "w") as lock:
    fcntl.flock(lock, fcntl.LOCK_EX)
    data = json.loads(config.read_text(encoding="utf-8")) if config.exists() else {}
    projects = data.get("projects") or {}
    project = projects.get(workspace) or {}
    if project.get("hasTrustDialogAccepted") is not True:
        project["hasTrustDialogAccepted"] = True
        projects[workspace] = project
        data["projects"] = projects
        fd, temporary = tempfile.mkstemp(prefix=config.name + ".tmp.", dir=config.parent)
        try:
            with os.fdopen(fd, "w", encoding="utf-8") as output:
                json.dump(data, output, indent=2)
                output.write("\n")
            os.replace(temporary, config)
        finally:
            if os.path.exists(temporary):
                os.unlink(temporary)
PY
