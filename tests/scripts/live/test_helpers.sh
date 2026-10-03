#!/usr/bin/env bash
# Offline regressions for live-lab lifecycle helpers and gateway startup shims.
set -euo pipefail
repo_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
cd "$repo_dir"
python3 - <<'PYTHON'
import contextlib
import io
import os
from pathlib import Path
import runpy
import sys
import tempfile
import types
from unittest.mock import patch

sys.path.insert(0, "tests/live")
import taxiway_live as live

with tempfile.TemporaryDirectory(prefix="taxiway-helper-test-") as root:
    environment = {"TAXIWAY_CONTEXT": "e2e", "TAXIWAY_CONTEXT_ID": "fixture",
                   "TAXIWAY_RUNTIME_DIR": root}
    for key, subdir in (("TAXIWAY_LAB_STATE_DIR", "labs"), ("TAXIWAY_AUTH_DIR", "auth"),
                        ("TAXIWAY_PROXY_DIR", "proxy"), ("TAXIWAY_OBSERVABILITY_DIR", "observability")):
        environment[key] = str(Path(root) / subdir)

    calls = []
    def fake_command(argv, **kwargs):
        calls.append(argv)
        if argv[1] == "rm":
            raise RuntimeError("simulated cleanup failure")
        return b""

    with patch.dict(os.environ, environment), patch.object(live, "command", fake_command):
        original = ValueError("scenario failed")
        stderr = io.StringIO()
        try:
            with contextlib.redirect_stderr(stderr), live.temporary_lab("claude-code") as lab:
                raise original
        except ValueError as error:
            assert error is original
        else:
            raise AssertionError("original scenario error was lost")
        assert f"Cleanup failed for {lab}" in stderr.getvalue()
        assert calls[-1] == ["./taxiway", "rm", lab, "--yes"]
        try:
            with live.temporary_lab("claude-code"):
                pass
        except RuntimeError as error:
            assert str(error).startswith("Cleanup failed for live-test-")
        else:
            raise AssertionError("cleanup error after successful scenario was lost")
    with patch.dict(os.environ, environment), patch.object(live, "command", lambda argv, **kwargs: calls.append(argv) or b""), patch.object(live, "require_claude_auth"), patch.object(live, "propagate_claude_auth", side_effect=lambda source, target: calls.append(["copy-auth", target])):
        calls.clear()
        with live.temporary_lab("gastown", auth_lab="reference"):
            pass
        assert "--prepare-only" in calls[0], "agents started before credentials were propagated"
        assert calls[1][0] == "copy-auth"
        assert calls[2][1] == "run"
print("PASS: reference auth is propagated before the first orchestrator start")
print("PASS: cleanup preserves the original error and reports owned-lab failures")

# Exercise the actual guest scripts with isolated local homes, no credentials.
import json
import subprocess
with tempfile.TemporaryDirectory(prefix="taxiway-onboarding-test-") as root:
    homes = {name: Path(root) / name for name in ("source", "target")}
    for home in homes.values(): home.mkdir()
    source = {"hasCompletedOnboarding": True, "lastOnboardingVersion": "test-version",
              "oauthAccount": {"sentinel": "source-only"}, "projects": {"source-workspace": {}}}
    target = {"theme": "light", "oauthAccount": {"sentinel": "target-only"},
              "projects": {"target-workspace": {"hasTrustDialogAccepted": True}}}
    for name, config in (("source", source), ("target", target)):
        (homes[name] / ".claude.json").write_text(json.dumps(config))
    def local_guest(lab, script, *, data=None, **kwargs):
        env = dict(os.environ, HOME=str(homes[lab]))
        return subprocess.run(["bash", "-c", script], input=data, env=env,
                              capture_output=True, check=True).stdout
    with patch.object(live, "guest", local_guest):
        live.propagate_claude_onboarding("source", "target")
        path = homes["target"] / ".claude.json"
        merged = json.loads(path.read_text())
        assert merged == dict(target, hasCompletedOnboarding=True, lastOnboardingVersion="test-version")
        assert path.stat().st_mode & 0o777 == 0o600
        before = path.read_bytes()
        live.propagate_claude_onboarding("source", "target")
        assert path.read_bytes() == before
        source["hasCompletedOnboarding"] = False
        (homes["source"] / ".claude.json").write_text(json.dumps(source))
        try: live.propagate_claude_onboarding("source", "target")
        except RuntimeError: pass
        else: raise AssertionError("incomplete reference onboarding accepted")
        assert path.read_bytes() == before
print("PASS: onboarding copied safely; account, theme and workspace settings retained")

# Missing internal functions should report the required compatibility check.
for name in ("litellm", "litellm.integrations", "litellm.integrations.custom_logger",
             "litellm.llms", "litellm.llms.anthropic", "litellm.llms.anthropic.common_utils",
             "litellm.llms.chatgpt", "litellm.llms.chatgpt.responses",
             "litellm.llms.chatgpt.responses.transformation"):
    module = types.ModuleType(name)
    module.__path__ = []
    sys.modules[name] = module
sys.modules["litellm.integrations.custom_logger"].CustomLogger = type("CustomLogger", (), {})
sys.modules["litellm.llms.chatgpt.responses.transformation"].ChatGPTResponsesAPIConfig = type("ChatGPTResponsesAPIConfig", (), {})
for name in ("anthropic_protocol", "codex_session_mapper"):
    try:
        runpy.run_path(f"infra/gateway/litellm/callbacks/{name}.py")
    except RuntimeError as error:
        assert "shim incompatible with this LiteLLM version" in str(error)
        assert "protocol test" in str(error)
    else:
        raise AssertionError("missing shim incompatibility diagnostic")
print("PASS: incompatible LiteLLM internals produce actionable startup errors")
PYTHON
