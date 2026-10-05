#!/usr/bin/env bash
# Offline regressions for live-lab lifecycle helpers and gateway startup shims.
set -euo pipefail
repo_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
cd "$repo_dir"
python3 - <<'PYTHON'
import contextlib
import io
import json
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
    with patch.dict(os.environ, environment), patch.object(live, "command", lambda argv, **kwargs: calls.append(argv) or b""), patch.object(live, "require_claude_auth"), patch.object(live, "_claude_onboarding"), patch.object(live, "propagate_claude_auth", side_effect=lambda source, target: calls.append(["copy-auth", target])):
        calls.clear()
        with live.temporary_lab("gastown", auth_lab="reference"):
            pass
        assert "--prepare-only" in calls[0], "agents started before credentials were propagated"
        assert calls[1][0] == "copy-auth"
        assert calls[2][1] == "run"
print("PASS: reference auth is propagated before the first orchestrator start")
print("PASS: cleanup preserves the original error and reports owned-lab failures")

with patch.dict(os.environ, environment), patch.object(live, 'command', return_value=b'') as commands:
    with live.temporary_lab('claude-code', driver='lima', prepare_only=True):
        pass
    assert '--prepare-only' in commands.call_args_list[0].args[0]
    assert not any(call.args[0][1] == 'run' for call in commands.call_args_list)
print('PASS: prepared Lima targets do not start unauthenticated agent roles')

# Rejected native auth must stop before provisioning, even with a credential file.
with patch.dict(os.environ, environment), patch.object(live, "guest", return_value=b"auth\n"), patch.object(live, "lab_ref", return_value={"driver": "docker"}), patch.object(live, "command") as provision:
    try:
        with live.temporary_lab("claude-code", auth_lab="reference"):
            pass
    except RuntimeError as error:
        assert "claude auth login" in str(error)
        assert "docker exec -it" in str(error)
    else:
        raise AssertionError("expired reference accepted before provisioning")
    provision.assert_not_called()
print("PASS: expired reference rejected before provisioning")

# Execute the native probe itself with a fixture client (no real credentials).
import subprocess
with tempfile.TemporaryDirectory(prefix="taxiway-preflight-test-") as root:
    home = Path(root)
    (home / '.claude').mkdir()
    (home / '.claude/.credentials.json').write_text('{}')
    native = home / 'claude'
    native.write_text('''#!/usr/bin/env python3
import os, sys
assert not any(key.startswith('ANTHROPIC_') for key in os.environ)
assert '--tools' in sys.argv and '--no-session-persistence' in sys.argv
assert sys.argv[sys.argv.index('--output-format') + 1] == 'json'
assert sys.argv[sys.argv.index('--mcp-config') + 1] == '{"mcpServers":{}}'
print(os.environ['FIXTURE_RESPONSE'])
sys.exit(int(os.environ['FIXTURE_EXIT']))
''')
    native.chmod(0o700)
    response, exitcode = '', 1
    def probe_guest(lab, script, **kwargs):
        env = dict(os.environ, HOME=root, PATH=root + os.pathsep + os.environ['PATH'],
                   FIXTURE_RESPONSE=response, FIXTURE_EXIT=str(exitcode),
                   ANTHROPIC_API_KEY='fixture-secret', ANTHROPIC_BASE_URL='fixture-gateway')
        return subprocess.run(['bash', '-c', script], env=env, capture_output=True, check=True).stdout
    with patch.object(live, 'guest', probe_guest), patch.object(live, 'lab_ref', return_value={'driver': 'lima'}), patch.object(live, 'runtime_id', return_value='fixture-reference'):
        response, exitcode = json.dumps({'type': 'result', 'subtype': 'success', 'is_error': False,
                                        'usage': {'output_tokens': 421}, 'result': 'A different answer',
                                        'modelUsage': {'fixture-model': {'permission': 'metadata-only'}}}), 0
        live.require_claude_auth('reference')
        # Current native startup/transport failures may precede JSON serialization.
        for response, category in [('Not logged in fixture-secret', 'native login'),
                                    ('Network connection timed out fixture-secret', '(network)'),
                                    ('Unknown option fixture-secret', '(setup)')]:
            exitcode = 1
            try: live.require_claude_auth('reference')
            except RuntimeError as error:
                assert category in str(error), str(error)
                assert 'fixture-secret' not in str(error)
            else: raise AssertionError('native plaintext failure accepted')
        for response, category in [('OAuth token has expired fixture-secret', 'native login'),
                                    ('Network connection timed out fixture-secret', '(network)'),
                                    ('Failed to refresh token: Network connection timed out fixture-secret', '(network)'),
                                    ('Failed to refresh token: HTTP 503 Service unavailable fixture-secret', '(provider)'),
                                    ('Invalid MCP configuration fixture-secret', '(setup)'),
                                    ('Model not available fixture-secret', '(model)'),
                                    ('Service overloaded fixture-secret', '(provider)')]:
            response = json.dumps({'type': 'result', 'subtype': 'error_during_execution',
                                   'is_error': True, 'errors': [response]})
            exitcode = 1
            try: live.require_claude_auth('reference')
            except RuntimeError as error:
                assert category in str(error), str(error)
                assert 'fixture-secret' not in str(error)
                assert ('claude auth login' in str(error)) == (category == 'native login')
                if category == 'native login': assert 'limactl shell fixture-reference' in str(error)
            else: raise AssertionError('native failure accepted')
        for payload, category, native_exit in [
            ({'type': 'result', 'subtype': 'error_max_budget_usd', 'is_error': True,
              'result': 'LIVE_AUTH_OK', 'usage': {'output_tokens': 10}}, '(provider)', 0),
            ({'type': 'result', 'subtype': 'error_max_turns', 'is_error': True,
              'errors': ['LIVE_AUTH_OK fixture-secret']}, '(provider)', 0),
            ({'type': 'result', 'subtype': 'error_during_execution', 'is_error': True,
              'errors': ['Invalid OAuth token fixture-secret']}, 'native login', 0),
            ({'type': 'result', 'subtype': 'success', 'is_error': False,
              'usage': {'output_tokens': 0}, 'result': 'LIVE_AUTH_OK'}, '(provider)', 0),
            ({'type': 'result', 'subtype': 'success', 'is_error': False,
              'usage': {'output_tokens': True}, 'result': 'LIVE_AUTH_OK'}, '(provider)', 0),
            ({'type': 'result', 'subtype': 'success', 'is_error': False,
              'usage': {'output_tokens': '1'}, 'result': 'LIVE_AUTH_OK'}, '(provider)', 0),
            ({'type': 'result', 'subtype': 'success', 'is_error': False,
              'usage': {'output_tokens': 1}, 'result': 'LIVE_AUTH_OK'}, '(provider)', 1),
            (None, '(provider)', 0),
            ([], '(provider)', 0),
        ]:
            response, exitcode = json.dumps(payload), native_exit
            try: live.require_claude_auth('reference')
            except RuntimeError as error:
                assert category in str(error), str(error)
                assert 'fixture-secret' not in str(error)
            else: raise AssertionError('invalid native result accepted')
        response, exitcode = 'malformed JSON fixture-secret', 0
        try: live.require_claude_auth('reference')
        except RuntimeError as error:
            assert '(provider)' in str(error) and 'fixture-secret' not in str(error)
        else: raise AssertionError('malformed native output accepted')
        for response in ('LIVE_AUTH_OK fixture-secret', 'Not logged in fixture-secret'):
            exitcode = 0
            try: live.require_claude_auth('reference')
            except RuntimeError as error:
                assert '(provider)' in str(error) and 'native login' not in str(error)
                assert 'fixture-secret' not in str(error)
            else: raise AssertionError('unstructured successful output accepted')
print('PASS: native request distinguishes auth/network/model/provider; secrets withheld')

with patch.dict(os.environ, environment), patch.object(live, 'require_claude_auth'), patch.object(live, '_claude_onboarding', side_effect=RuntimeError('Complete onboarding')), patch.object(live, 'propagate_claude_auth'), patch.object(live, 'command', return_value=b'') as commands:
    try:
        with live.temporary_lab('claude-code', auth_lab='reference'):
            pass
    except RuntimeError as error: assert str(error) == 'Complete onboarding'
    else: raise AssertionError('incomplete reference onboarding accepted')
    commands.assert_not_called()

# Expiry between initial preflight and propagation fails once and removes only the target.
with patch.dict(os.environ, environment), patch.object(live, 'require_claude_auth'), patch.object(live, '_claude_onboarding'), patch.object(live, 'propagate_claude_auth', side_effect=RuntimeError('reference requires native login')), patch.object(live, 'command', return_value=b'') as commands:
    try:
        with live.temporary_lab('claude-code', auth_lab='reference'):
            raise AssertionError('scenario started with rejected credentials')
    except RuntimeError as error:
        assert str(error) == 'reference requires native login'
    else: raise AssertionError('mid-setup rejection ignored')
    assert len(commands.call_args_list) == 2, 'unbounded recovery or replay'
    created = commands.call_args_list[0].args[0][2]
    assert commands.call_args_list[1].args[0] == ['./taxiway', 'rm', created, '--yes']
    assert created != 'reference'
print('PASS: mid-setup auth rejection is bounded and preserves the reference')

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
    native = Path(root) / 'claude'
    native.write_text('''#!/usr/bin/env python3
import json
from pathlib import Path
path = Path.home() / '.claude/.credentials.json'
credential = json.loads(path.read_text())
credential['claudeAiOauth']['accessToken'] = 'fixture-refreshed'
path.write_text(json.dumps(credential))
print(json.dumps({'type': 'result', 'subtype': 'success', 'is_error': False,
                  'usage': {'output_tokens': 1}, 'result': 'LIVE_AUTH_OK'}))
''')
    native.chmod(0o700)
    (homes['source'] / '.claude').mkdir()
    (homes['source'] / '.claude/.credentials.json').write_text(json.dumps({'claudeAiOauth': {'accessToken': 'fixture-expired', 'refreshToken': 'fixture-refresh'}}))
    def local_guest(lab, script, *, data=None, **kwargs):
        env = dict(os.environ, HOME=str(homes[lab]), PATH=root + os.pathsep + os.environ['PATH'])
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
        live.propagate_claude_auth('source', 'target')
        copied = homes['target'] / '.claude/.credentials.json'
        assert json.loads(copied.read_text())['claudeAiOauth']['accessToken'] == 'fixture-refreshed'
        assert copied.stat().st_mode & 0o777 == 0o600
        assert copied.parent.stat().st_mode & 0o777 == 0o700
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
