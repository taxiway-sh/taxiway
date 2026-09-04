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

project_header="$(python3 - "$workspace_path" <<'PY'
import json
import sys

print(f"[projects.{json.dumps(sys.argv[1])}]")
PY
)"

if [[ -f "$config_path" ]]; then
    CODEX_PROJECT_HEADER="$project_header" awk '
        BEGIN {
            target = ENVIRON["CODEX_PROJECT_HEADER"]
            in_target = 0
            found_target = 0
            wrote_trust = 0
        }
        $0 == target {
            in_target = 1
            found_target = 1
            wrote_trust = 0
            print
            next
        }
        /^\[/ {
            if (in_target && !wrote_trust) {
                print "trust_level = \"trusted\""
            }
            in_target = 0
        }
        in_target && /^[[:space:]]*trust_level[[:space:]]*=/ {
            if (!wrote_trust) {
                print "trust_level = \"trusted\""
                wrote_trust = 1
            }
            next
        }
        { print }
        END {
            if (in_target && !wrote_trust) {
                print "trust_level = \"trusted\""
            } else if (!found_target) {
                print ""
                print target
                print "trust_level = \"trusted\""
            }
        }
    ' "$config_path" > "$tmp_config"
else
    printf '%s\ntrust_level = "trusted"\n' "$project_header" > "$tmp_config"
fi

python3 - "$tmp_config" <<'PY'
import sys
import tomllib

with open(sys.argv[1], "rb") as config_file:
    tomllib.load(config_file)
PY

chmod 0600 "$tmp_config"
mv "$tmp_config" "$config_path"
trap - EXIT

