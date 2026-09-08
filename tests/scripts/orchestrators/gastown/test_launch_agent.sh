#!/usr/bin/env bash
# Exercise the Gastown launcher and the real lifecycle trust hook.
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
python3 - "$SCRIPT_DIR/../../../../" <<'PY'
from concurrent.futures import ThreadPoolExecutor
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

ROOT = Path(sys.argv.pop()).resolve()
HOOK = str(ROOT / "agents/claude-code/trust-workspace.sh")
LAUNCHER = str(ROOT / "orchestrators/gastown/launch-agent.sh")


class TrustExecTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.base = Path(self.temp.name).resolve()
        self.user_home = self.base / "home"
        self.hq = self.base / "town"
        self.workspace = self.hq / 'rig/crew/person with "quotes"'
        self.user_home.mkdir()
        self.workspace.mkdir(parents=True)
        self.config = self.user_home / ".claude.json"
        self.env = dict(os.environ, HOME=str(self.user_home),
                        TAXIWAY_WORKSPACE_TRUST_ROOT=str(self.hq),
                        TAXIWAY_WORKSPACE_TRUST_PATH="/stale/path",
                        TRUST_TEST_ENV="preserved")

    def run_hook(self, args, cwd=None, env=None):
        return subprocess.run(["bash", LAUNCHER, *args], cwd=cwd or self.workspace,
                              env=env or self.env, capture_output=True, text=True)

    def test_exec_trusts_actual_cwd_and_preserves_args_env_pid_and_exit(self):
        link = self.base / "workspace-link"
        link.symlink_to(self.workspace, target_is_directory=True)
        program = '''import json, os, pathlib, sys
c = json.loads((pathlib.Path.home()/".claude.json").read_text())
assert c["projects"][os.getcwd()]["hasTrustDialogAccepted"] is True
assert "/stale/path" not in c["projects"]
assert os.environ["TRUST_TEST_ENV"] == "preserved"
assert sys.argv[1:] == ["--model", "model with spaces", "$(not-a-command)"]
print(os.getpid())
sys.exit(23)
'''
        process = subprocess.Popen(["bash", LAUNCHER, sys.executable, "-c", program,
                                    "--model", "model with spaces", "$(not-a-command)"],
                                   cwd=link, env=self.env, stdout=subprocess.PIPE,
                                   stderr=subprocess.PIPE, text=True)
        stdout, stderr = process.communicate(timeout=10)
        self.assertEqual(process.returncode, 23, stderr)
        self.assertEqual(stdout.strip(), str(process.pid))

    def test_rejects_outside_root_and_symlink_escape(self):
        outside = self.base / "town-other"
        outside.mkdir()
        link = self.hq / "escape"
        link.symlink_to(outside, target_is_directory=True)
        for cwd in (outside, link):
            result = self.run_hook(["true"], cwd=cwd)
            self.assertNotEqual(result.returncode, 0)
        self.assertFalse(self.config.exists())

    def test_rejects_missing_command_or_root_and_unknown_options(self):
        for args in ([], ["--unknown"]):
            self.assertNotEqual(self.run_hook(args).returncode, 0)
        env = dict(self.env)
        env.pop("TAXIWAY_WORKSPACE_TRUST_ROOT")
        self.assertNotEqual(self.run_hook(["true"], env=env).returncode, 0)
        for root in ("/", "relative", str(self.base / "missing")):
            env["TAXIWAY_WORKSPACE_TRUST_ROOT"] = root
            self.assertNotEqual(self.run_hook(["true"], env=env).returncode, 0)
        self.assertFalse(self.config.exists())

    def test_already_trusted_config_is_not_rewritten(self):
        original = json.dumps({"projects": {str(self.workspace): {
            "hasTrustDialogAccepted": True, "allowedTools": ["Read"]}}})
        self.config.write_text(original)
        before = self.config.stat().st_mtime_ns
        self.assertEqual(self.run_hook(["true"]).returncode, 0)
        self.assertEqual(self.config.read_text(), original)
        self.assertEqual(self.config.stat().st_mtime_ns, before)

    def test_invalid_config_does_not_launch_command_or_replace_file(self):
        self.config.write_text("not json")
        marker = self.base / "launched"
        result = self.run_hook(["touch", str(marker)])
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(self.config.read_text(), "not json")
        self.assertFalse(marker.exists())

    def test_dynamic_workspaces_and_concurrent_updates_are_preserved(self):
        self.config.write_text(json.dumps({"theme": "dark", "projects": {
            "/existing": {"allowedTools": ["Read"]}}}))
        paths = [self.hq / f"rig/polecats/worker-{i}" for i in range(12)]
        for path in paths:
            path.mkdir(parents=True)
        def trust(path):
            # Lifecycle provisioning and runtime launches share the same lock.
            if paths.index(path) % 2:
                return subprocess.run(["bash", HOOK], env=dict(self.env, TAXIWAY_WORKSPACE_TRUST_PATH=str(path)), capture_output=True, text=True)
            return self.run_hook(["true"], cwd=path)
        with ThreadPoolExecutor(max_workers=12) as pool:
            results = list(pool.map(trust, paths))
        for result in results:
            self.assertEqual(result.returncode, 0, result.stderr)
        config = json.loads(self.config.read_text())
        self.assertEqual(config["theme"], "dark")
        self.assertEqual(config["projects"]["/existing"], {"allowedTools": ["Read"]})
        for path in paths:
            self.assertTrue(config["projects"][str(path)]["hasTrustDialogAccepted"])
        self.assertEqual(self.config.stat().st_mode & 0o777, 0o600)

    def test_lifecycle_hook_does_not_launch_commands(self):
        marker = self.base / "unexpected-launch"
        result = subprocess.run(["bash", HOOK, "--exec", "touch", str(marker)],
                                cwd=self.workspace, env=self.env, capture_output=True, text=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(marker.exists())


unittest.main()
PY
