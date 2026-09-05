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
exit 0
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

PATH="$fake_bin:$PATH" \
HOME="$home" \
TAXIWAY_LITELLM_API_KEY="test-key" \
TAXIWAY_LITELLM_BASE_URL="http://gateway.test:4000" \
TAXIWAY_WORKSPACE_DIR="$workspace" \
bash "$START_SH" >/dev/null

python3 - "$home/.codex/config.toml" "$workspace" <<'PY'
import sys
import tomllib

with open(sys.argv[1], "rb") as config_file:
    config = tomllib.load(config_file)

assert config["projects"][sys.argv[2]]["trust_level"] == "trusted"
assert config["projects"][sys.argv[2]]["marker"] == "preserve-me"
assert config["model_provider"] == "taxiway-litellm"
assert config["model_providers"]["taxiway-litellm"]["requires_openai_auth"] is False
PY

echo "PASS: Codex start preserves workspace trust while configuring LiteLLM"
