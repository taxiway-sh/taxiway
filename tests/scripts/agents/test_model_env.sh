#!/usr/bin/env bash
# Model aliases and picker settings must survive tmux/Gas Town handoffs.
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
python3 - "$SCRIPT_DIR/../../../" <<'PY'
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

ROOT = Path(sys.argv.pop()).resolve()


class ModelEnvironmentTests(unittest.TestCase):
    def test_aliases_and_picker_persist_without_replacing_other_settings(self):
        with tempfile.TemporaryDirectory() as home:
            settings = Path(home) / "managed/settings.json"
            settings.parent.mkdir()
            settings.write_text(json.dumps({"permissions": {"allow": ["Read"]}}))
            fake_bin = Path(home) / "bin"
            fake_bin.mkdir()
            cli = fake_bin / "claude"
            cli.write_text("#!/bin/sh\nprintf '2.1.284 (Claude Code)\\n'\n")
            cli.chmod(0o755)
            env = dict(os.environ, HOME=home, PATH=str(fake_bin)+os.pathsep+os.environ["PATH"],
                       ANTHROPIC_DEFAULT_OPUS_MODEL="claude-opus-5-5",
                       ANTHROPIC_DEFAULT_SONNET_MODEL="claude-sonnet-5-5",
                       ANTHROPIC_DEFAULT_HAIKU_MODEL="claude-haiku-4-5-20251001",
                       TAXIWAY_CLAUDE_AVAILABLE_MODELS=json.dumps(["claude-opus-5-5", "claude-haiku-4-5-20251001"]))
            script = 'source "$1"; claude_code_write_env true false "$2"; export ANTHROPIC_DEFAULT_OPUS_MODEL=stale; claude_code_load_env; printf "%s" "$ANTHROPIC_DEFAULT_OPUS_MODEL"'
            result = subprocess.run(["bash", "-c", script, "test", str(ROOT / "agents/claude-code/env.sh"), str(settings)],
                                    env=env, text=True, capture_output=True)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(result.stdout, "claude-opus-5-5")
            policy = json.loads(settings.read_text())
            self.assertEqual(policy["permissions"], {"allow": ["Read"]})
            self.assertEqual(policy["availableModels"], json.loads(env["TAXIWAY_CLAUDE_AVAILABLE_MODELS"]))
            self.assertTrue(policy["enforceAvailableModels"])
            self.assertEqual(policy["availableModelsMatch"], "exact")
            self.assertEqual(settings.stat().st_mode & 0o777, 0o644)


unittest.main()
PY
