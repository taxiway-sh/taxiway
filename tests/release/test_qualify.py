"""Exercise the recipe helper with real subprocesses and isolated Git repos."""
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import time
import unittest

RUNNER = Path(__file__).with_name("qualify.py")


class QualificationTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.repo = Path(self.tmp.name) / "repo"
        self.repo.mkdir()
        self.report = Path(self.tmp.name) / "evidence"
        self.git("init", "-q")
        self.git("-c", "user.name=Test", "-c", "user.email=test@example.invalid",
                 "commit", "--allow-empty", "-qm", "candidate")
        self.sha = self.git("rev-parse", "HEAD").stdout.strip()

    def git(self, *args):
        return subprocess.run(["git", "-c", "commit.gpgsign=false", *args], cwd=self.repo, text=True,
                              capture_output=True, check=True)

    def cli(self, *args):
        return subprocess.run([sys.executable, str(RUNNER), "--report", str(self.report),
                               *args], cwd=self.repo, text=True, capture_output=True)

    def start(self):
        result = self.cli("init", "--candidate", self.sha)
        self.assertEqual(result.returncode, 0, result.stderr)

    def check_result(self, status):
        data = json.loads((self.report / "report.json").read_text())
        self.assertEqual(data["checks"]["core"]["status"], status)

    def test_initial_report_never_implies_complete(self):
        self.start()
        self.assertNotEqual(self.cli("report").returncode, 0)
        self.assertIn("NOT_EXECUTED", (self.report / "report.md").read_text())
        self.assertEqual(self.git("tag").stdout, "")

    def test_success_and_failure_propagate(self):
        self.start()
        self.assertEqual(self.cli("run", "core", "--", sys.executable, "-c", "print('ok')").returncode, 0)
        self.check_result("PASS")
        self.assertNotEqual(self.cli("run", "core", "--", sys.executable, "-c", "raise SystemExit(7)").returncode, 0)
        self.check_result("FAIL")
        self.assertEqual(len(list(self.report.glob("core-*.log"))), 2)

    def test_timeout_and_skips_do_not_pass(self):
        self.start()
        self.assertNotEqual(self.cli("run", "core", "--timeout", "0.1", "--", sys.executable,
                                    "-c", "import time; time.sleep(10)").returncode, 0)
        self.check_result("FAIL")
        self.assertNotEqual(self.cli("run", "core", "--", sys.executable,
                                    "-c", "print('--- SKIP: TestDocker (0.00s)')").returncode, 0)
        self.check_result("NOT_EXECUTED")

    def test_timeout_kills_child_even_when_parent_exits_on_term(self):
        self.start()
        pidfile = Path(self.tmp.name) / "child.pid"
        survived = Path(self.tmp.name) / "survived"
        child = ("import signal,time; from pathlib import Path; "
                 "signal.signal(signal.SIGTERM,signal.SIG_IGN); time.sleep(1); "
                 f"Path({str(survived)!r}).write_text('alive'); time.sleep(30)")
        parent = ("import subprocess,sys,time; from pathlib import Path; "
                  f"p=subprocess.Popen([sys.executable,'-c',{child!r}]); "
                  f"Path({str(pidfile)!r}).write_text(str(p.pid)); time.sleep(30)")
        self.assertNotEqual(self.cli("run", "core", "--timeout", "0.5", "--", sys.executable,
                                    "-c", parent).returncode, 0)
        child_pid = int(pidfile.read_text())
        time.sleep(1)
        try:
            self.assertFalse(survived.exists(), "Child survived timeout after its parent exited")
        finally:
            try:
                os.kill(child_pid, 9)
            except ProcessLookupError:
                pass

    def test_changed_candidate_and_dirty_tree_rejected(self):
        self.start()
        (self.repo / "untracked").write_text("new behavior")
        self.assertNotEqual(self.cli("run", "core", "--", "true").returncode, 0)
        (self.repo / "untracked").unlink()
        self.git("-c", "user.name=Test", "-c", "user.email=test@example.invalid",
                 "commit", "--allow-empty", "-qm", "other")
        self.assertNotEqual(self.cli("report").returncode, 0)

    def test_external_evidence_requires_exact_sha_and_existing_file(self):
        self.start()
        proof = Path(self.tmp.name) / "proof.txt"
        proof.write_text("Reviewed workflow jobs, all successful; no skips.")
        args = ("record", "core", "--status", "PASS", "--evidence", str(proof))
        self.assertNotEqual(self.cli(*args, "--candidate", "old-sha").returncode, 0)
        self.assertNotEqual(self.cli(*args, "--candidate", self.sha, "--note", "").returncode, 0)
        self.assertEqual(self.cli(*args, "--candidate", self.sha, "--note", "Manual workflow inspection").returncode, 0)
        self.check_result("PASS")

    def test_report_must_be_outside_checkout_and_not_reinitialized(self):
        self.assertNotEqual(self.cli("--report", str(self.repo / "report"), "init", "--candidate", self.sha).returncode, 0)
        self.start()
        self.assertNotEqual(self.cli("init", "--candidate", self.sha).returncode, 0)

    def test_same_report_cannot_run_two_commands_concurrently(self):
        self.start()
        first = subprocess.Popen([sys.executable, str(RUNNER), "--report", str(self.report),
                                  "run", "core", "--", sys.executable, "-c", "import time; time.sleep(1)"],
                                 cwd=self.repo, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        try:
            for _ in range(100):
                if list(self.report.glob("core-*.log")):
                    break
                time.sleep(0.01)
            second = self.cli("run", "scripts", "--", "true")
            self.assertNotEqual(second.returncode, 0)
            self.assertIn("active", second.stderr)
        finally:
            first.communicate(timeout=10)

    def test_post_phase_requires_an_unchanged_real_tag(self):
        self.git("branch", "v1.2.3")
        self.assertNotEqual(self.cli("init", "--phase", "post", "--candidate", "v1.2.3").returncode, 0)
        self.git("tag", "v1.2.3")
        self.assertEqual(self.cli("init", "--phase", "post", "--candidate", "v1.2.3").returncode, 0)
        self.git("tag", "-d", "v1.2.3")
        self.assertNotEqual(self.cli("report").returncode, 0)
        self.git("-c", "user.name=Test", "-c", "user.email=test@example.invalid",
                 "commit", "--allow-empty", "-qm", "other")
        self.git("tag", "v1.2.3")
        self.git("checkout", "-q", self.sha)
        self.assertNotEqual(self.cli("run", "published-assets", "--", "true").returncode, 0)


if __name__ == "__main__":
    unittest.main()
