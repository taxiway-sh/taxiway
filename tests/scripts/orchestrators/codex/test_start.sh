#!/usr/bin/env bash
# Contract test that Codex start preserves workspace trust configured earlier.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
START_SH="$SCRIPT_DIR/../../../../orchestrators/codex/start.sh"

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT
home="$tmp_dir/home"
fake_bin="$tmp_dir/bin"
workspace="$tmp_dir/workspace"
mkdir -p "$home/.codex" "$fake_bin" "$workspace"

cat > "$home/.codex/config.toml" <<EOF
[projects."$workspace"]
trust_level = "trusted"
marker = "preserve-me"
EOF

cat > "$fake_bin/tmux" <<'EOF'
#!/usr/bin/env bash
if [[ "${1:-}" == "has-session" ]]; then
    exit 1
fi
bash -c "${!#}"
EOF
chmod +x "$fake_bin/tmux"

cat > "$fake_bin/mkdir" <<'EOF'
#!/usr/bin/env bash
if [[ "$*" == "-p /lab/work" ]]; then
    exit 0
fi
exec /bin/mkdir "$@"
EOF
chmod +x "$fake_bin/mkdir"

cat > "$fake_bin/codex" <<'EOF'
#!/usr/bin/env python3
import json, os, sys
if sys.argv[1:] == ["debug", "models", "--bundled"]:
    print(json.dumps({"models": [{"slug": "test-selected-codex-model", "context_window": 12345,
        "upgrade": {"id": "unselected-model"}, "supported_reasoning_levels": [{"effort": "high"}]}]}))
    sys.exit(0)
with open(os.environ["TEST_RESULT"], "a") as out:
    out.write(json.dumps(sys.argv[1:]) + "\n")
# Exercise the fresh-start fallback as well as resume.
sys.exit(1 if sys.argv[1:2] == ["resume"] else 0)
EOF
chmod +x "$fake_bin/codex"

for _ in 1 2; do
TEST_RESULT="$tmp_dir/commands.jsonl" \
PATH="$fake_bin:$PATH" \
HOME="$home" \
TAXIWAY_LITELLM_API_KEY="test-key" \
TAXIWAY_LITELLM_BASE_URL="http://gateway.test:4000" \
TAXIWAY_SET_MODEL="test-selected-codex-model" \
TAXIWAY_WORKSPACE_DIR="$workspace" \
bash "$START_SH" >/dev/null
done

python3 - "$home/.codex/config.toml" "$workspace" "$tmp_dir/commands.jsonl" <<'PY'
import sys
import json
import tomllib

with open(sys.argv[1], "rb") as config_file:
    config = tomllib.load(config_file)

commands = [json.loads(line) for line in open(sys.argv[3])]
assert commands == [["resume", "--last", "--dangerously-bypass-approvals-and-sandbox"], ["--dangerously-bypass-approvals-and-sandbox"]] * 2, commands

assert config["projects"][sys.argv[2]]["trust_level"] == "trusted"
assert config["projects"][sys.argv[2]]["marker"] == "preserve-me"
assert config["model_provider"] == "taxiway-litellm"
assert config["approval_policy"] == "never"
assert config["sandbox_mode"] == "danger-full-access"
assert config["model"] == "test-selected-codex-model"
catalog = json.load(open(config["model_catalog_json"]))
assert catalog["models"][0]["upgrade"] is None
assert catalog["models"][0]["context_window"] == 12345
assert catalog["models"][0]["supported_reasoning_levels"] == [{"effort": "high"}]
assert config["model_providers"]["taxiway-litellm"]["requires_openai_auth"] is False
PY

echo "PASS: Codex start preserves workspace trust while configuring LiteLLM"
