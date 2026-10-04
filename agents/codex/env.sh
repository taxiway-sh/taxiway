#!/usr/bin/env bash
# The orchestrator resolves its settings and owns ~/.codex/config.toml; this
# helper provides the agent policy and native gateway model catalog settings.

# codex_update_policy_config <codex-version>
# A pinned Codex must not offer to replace itself with another release.
codex_update_policy_config() {
    if [[ -n "${1:-}" && "$1" != "latest" ]]; then
        printf 'check_for_update_on_startup = false\n'
    fi
}

# Taxiway owns the selected gateway model. Preserve native model capabilities,
# but do not offer migrations to a different model before accepting a request.
codex_gateway_model_catalog() (
    set -euo pipefail
    local destination="$HOME/.codex/taxiway-models.json"
    local native
    native="$(mktemp)"
    trap 'rm -f "$native"' EXIT
    codex debug models --bundled > "$native"
    python3 - "$native" "$destination" <<'PY'
import json
import os
import sys
import tempfile
from pathlib import Path

catalog = json.loads(Path(sys.argv[1]).read_text())
models = catalog.get("models")
if not isinstance(models, list) or not models or not all(isinstance(model, dict) for model in models):
    raise ValueError("Codex did not return its native model catalog")
for model in models:
    model["upgrade"] = None
destination = Path(sys.argv[2])
fd, temporary = tempfile.mkstemp(prefix=destination.name + ".", dir=destination.parent)
try:
    with os.fdopen(fd, "w") as output:
        json.dump(catalog, output)
        output.write("\n")
    os.replace(temporary, destination)
finally:
    if os.path.exists(temporary):
        os.unlink(temporary)
print("model_catalog_json = " + json.dumps(str(destination)))
PY
)
