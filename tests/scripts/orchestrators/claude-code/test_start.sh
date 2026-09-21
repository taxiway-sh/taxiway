#!/usr/bin/env bash
# Exercise the real start script and launcher with an existing tmux environment.
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
python3 - "$SCRIPT_DIR/../../../../" <<'PY'
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

ROOT = Path(sys.argv.pop()).resolve()


class StartTests(unittest.TestCase):
    def test_restart_reloads_settings_inside_tmux(self):
        with tempfile.TemporaryDirectory(prefix="claude start '") as temp:
            base = Path(temp)
            user_home = base / "home"
            settings = user_home / ".config/taxiway/agents/claude-code.env"
            settings.parent.mkdir(parents=True)
            fake_bin = base / "bin"
            fake_bin.mkdir()
            programs = {
                "mkdir": '#!/bin/bash\nif [[ "$*" != "-p /lab/work" ]]; then exec /bin/mkdir "$@"; fi\n',
                "tmux": '''#!/usr/bin/env python3
import os, subprocess, sys
if sys.argv[1] in ("has-session", "kill-session"):
    sys.exit(0)
assert sys.argv[1] == "new-session"
# Simulate a server with old settings, then execute the actual session command.
env = dict(os.environ, ENABLE_TOOL_SEARCH="stale", ENABLE_CLAUDEAI_MCP_SERVERS="stale")
sys.exit(subprocess.call(["bash", "-c", sys.argv[-1]], env=env))
''',
                "claude": '''#!/usr/bin/env python3
import json, os, sys
with open(os.environ["TEST_RESULT"], "w") as out:
    json.dump([os.environ.get("ENABLE_TOOL_SEARCH"), os.environ.get("ENABLE_CLAUDEAI_MCP_SERVERS"), sys.argv[1:]], out)
''',
            }
            for name, content in programs.items():
                path = fake_bin / name
                path.write_text(content)
                path.chmod(0o755)
            result_path = base / "result.json"
            model = 'model with "quotes" and spaces'
            env = dict(os.environ, HOME=str(user_home), PATH=str(fake_bin)+os.pathsep+os.environ["PATH"],
                       TAXIWAY_WORKSPACE_DIR=str(base), TAXIWAY_SET_MODEL=model,
                       TAXIWAY_LITELLM_API_KEY="test-only-key", TEST_RESULT=str(result_path))
            for values, expected in [
                ({}, ["true", "false"]),
                ({"TAXIWAY_SET_TOOL_SEARCH": "false", "TAXIWAY_SET_CLAUDEAI_MCP_SERVERS": "false"}, ["false", "false"]),
                ({"TAXIWAY_SET_TOOL_SEARCH": "auto:5", "TAXIWAY_SET_CLAUDEAI_MCP_SERVERS": "true"}, ["auto:5", "true"]),
                ({"TAXIWAY_SET_TOOL_SEARCH": "literal 'quotes' $(false)"}, ["literal 'quotes' $(false)", "false"]),
                ({}, ["true", "false"]),  # --clear-set removes both persisted overrides.
            ]:
                env.pop("TAXIWAY_SET_TOOL_SEARCH", None)
                env.pop("TAXIWAY_SET_CLAUDEAI_MCP_SERVERS", None)
                env.update(values)
                result = subprocess.run(["bash", str(ROOT / "orchestrators/claude-code/start.sh")],
                                        env=env, capture_output=True, text=True)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(json.loads(result_path.read_text()), expected + [["--model", model]])
                self.assertEqual(settings.stat().st_mode & 0o777, 0o600)

    def test_write_failure_stops_start(self):
        with tempfile.TemporaryDirectory() as temp:
            user_home = Path(temp)
            (user_home / ".config").write_text("not a directory")
            env = dict(os.environ, HOME=temp, TAXIWAY_LITELLM_API_KEY="test-only-key")
            result = subprocess.run(["bash", str(ROOT / "orchestrators/claude-code/start.sh")],
                                    env=env, capture_output=True, text=True)
            self.assertNotEqual(result.returncode, 0)
            self.assertIn(".config", result.stderr)
            self.assertNotIn("Starting tmux", result.stdout)


unittest.main()
PY
