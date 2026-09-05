#!/usr/bin/env bash
# Mark a Taxiway-managed workspace as trusted by Codex.

set -euo pipefail

workspace_path="${TAXIWAY_WORKSPACE_TRUST_PATH:-}"
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

config_dir="${HOME}/.codex"
config_path="${config_dir}/config.toml"
mkdir -p "$config_dir"
tmp_config="$(mktemp "${config_path}.tmp.XXXXXX")"
trap 'rm -f "$tmp_config"' EXIT

python3 - "$workspace_path" "$config_path" > "$tmp_config" <<'PY'
import json
from pathlib import Path
import sys
import tomllib

workspace, filename = sys.argv[1:]
path = Path(filename)
source = path.read_bytes().decode("utf-8") if path.exists() else ""
config = tomllib.loads(source)
project = config.get("projects", {}).get(workspace, {})
if project.get("trust_level") == "trusted":
    sys.stdout.write(source)
    sys.exit(0)

# Locate complete TOML statements using the parser, not header spelling.
# Complete prefixes keep header-like text inside multiline strings/arrays
# from being mistaken for a real table. Preserve every unrelated byte.
lines = source.splitlines(keepends=True)
start = 0
section_start = section_end = None
trust_span = None
for end in range(1, len(lines) + 1):
    try:
        tomllib.loads("".join(lines[:end]))
    except tomllib.TOMLDecodeError:
        continue
    statement = "".join(lines[start:end])
    if statement.lstrip().startswith("["):
        if section_start is not None:
            section_end = start
            break
        if tomllib.loads(statement) == {"projects": {workspace: {}}}:
            section_start = end
    elif section_start is not None:
        if "trust_level" in tomllib.loads(statement):
            trust_span = (start, end)
    start = end

trust = 'trust_level = "trusted"\n'
if section_start is not None:
    if trust_span is None:
        offset = section_end if section_end is not None else len(lines)
        trust_span = (offset, offset)
    first, last = trust_span
    prefix = "".join(lines[:first])
    result = prefix + ("\n" if prefix and not prefix.endswith("\n") else "") + trust + "".join(lines[last:])
else:
    header = f"[projects.{json.dumps(workspace, ensure_ascii=False)}]"
    result = source + ("\n" if source else "") + header + "\n" + trust

# Fail without replacing the original file if its layout cannot be updated.
updated = tomllib.loads(result)
assert updated["projects"][workspace]["trust_level"] == "trusted"
sys.stdout.write(result)
PY

chmod 0600 "$tmp_config"
mv "$tmp_config" "$config_path"
trap - EXIT
