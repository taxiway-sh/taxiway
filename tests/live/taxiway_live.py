"""Shared helpers for opt-in live Taxiway scenarios; standard library only."""

import argparse
from contextlib import contextmanager
import json
import os
from pathlib import Path
import re
import shlex
import subprocess
import sys
import uuid


def validate_context():
    if os.environ.get("TAXIWAY_CONTEXT") not in ("dev", "e2e"):
        raise RuntimeError("Live tests require a dev or e2e context; run through direnv exec .")
    keys = ("TAXIWAY_CONTEXT_ID", "TAXIWAY_RUNTIME_DIR", "TAXIWAY_LAB_STATE_DIR",
            "TAXIWAY_AUTH_DIR", "TAXIWAY_PROXY_DIR", "TAXIWAY_OBSERVABILITY_DIR")
    for key in keys:
        if not os.environ.get(key):
            raise RuntimeError(f"{key} must be set; run through direnv exec .")
    if not re.fullmatch(r"[a-z0-9-]+", os.environ["TAXIWAY_CONTEXT_ID"]):
        raise RuntimeError("Invalid runtime context ID")
    root = Path(os.environ["TAXIWAY_RUNTIME_DIR"]).resolve()
    for key in keys[2:]:
        path = Path(os.environ[key]).resolve()
        if path == root or not path.is_relative_to(root):
            raise RuntimeError(f"{key} must be a directory below TAXIWAY_RUNTIME_DIR")


def command(argv, *, data=None, timeout=300):
    result = subprocess.run(argv, input=data, stdout=subprocess.PIPE,
                            stderr=subprocess.PIPE, timeout=timeout)
    if result.returncode:
        # Never echo output or argv: either can contain credentials.
        raise RuntimeError(f"{Path(argv[0]).name} exited {result.returncode}")
    return result.stdout


def lab_ref(lab):
    validate_context()
    if not re.fullmatch(r"[A-Za-z0-9_-]{1,48}", lab):
        raise RuntimeError("Invalid lab name")
    return json.loads((Path(os.environ["TAXIWAY_LAB_STATE_DIR"]) / lab / "ref.json").read_text())


def runtime_id(lab):
    lab_ref(lab)
    return f"taxiway-{os.environ['TAXIWAY_CONTEXT']}-{os.environ['TAXIWAY_CONTEXT_ID']}-{lab}"


def guest(lab, script, *, data=None, timeout=300, workdir="/lab/work"):
    ref = lab_ref(lab)
    runtime = runtime_id(lab)
    if ref["driver"] == "docker":
        argv = ["docker", "exec", "-i", "-u", "taxiway", "-e",
                "HOME=/home/taxiway", "-w", workdir, runtime]
    elif ref["driver"] == "lima":
        argv = ["limactl", "shell", "--workdir=" + workdir, runtime]
    else:
        raise RuntimeError("Unsupported lab driver")
    return command(argv + ["bash", "-lc", script], data=data, timeout=timeout)


def require_claude_auth(lab):
    """A native bounded request permits native refresh; only categories leave the guest."""
    status = guest(lab, '''python3 - <<'PY'
import os, pathlib, shutil, subprocess
if not shutil.which('claude'):
    print('setup')
    raise SystemExit
if not (pathlib.Path.home() / '.claude/.credentials.json').is_file():
    print('auth')
    raise SystemExit
env = dict(os.environ)
for key in list(env):
    if key.startswith('ANTHROPIC_') or key in ('CLAUDE_CODE_OAUTH_TOKEN', 'CLAUDE_CODE_USE_BEDROCK', 'CLAUDE_CODE_USE_VERTEX', 'CLAUDE_CODE_USE_FOUNDRY'):
        env.pop(key, None)
try:
    result = subprocess.run(['claude', '-p', 'Reply exactly LIVE_AUTH_OK.', '--model', 'claude-haiku-4-5-20251001', '--tools', '', '--strict-mcp-config', '--mcp-config', '{}', '--no-session-persistence', '--max-turns', '1', '--max-budget-usd', '0.10'], env=env, capture_output=True, timeout=90)
except subprocess.TimeoutExpired:
    print('network')
else:
    text = (result.stdout + result.stderr).decode(errors='replace').lower()
    if result.returncode == 0 and 'live_auth_ok' in text:
        print('ok')
    elif any(message in text for message in ('not logged in', 'please run /login', 'please run claude auth login', 'oauth token has expired', 'invalid oauth token', 'authentication_error', 'token has been revoked', 'invalid bearer token', 'failed to refresh token')):
        print('auth')
    elif any(message in text for message in ('connection refused', 'timed out', 'timeout', 'enotfound', 'econnreset', 'network')):
        print('network')
    elif any(message in text for message in ('model', 'permission', 'forbidden', 'not allowed')):
        print('model')
    else:
        print('provider')
PY''', timeout=120).strip()
    if status == b'ok':
        return
    if status == b'auth':
        ref = lab_ref(lab)
        runtime = runtime_id(lab)
        if ref['driver'] == 'docker':
            login = ['docker', 'exec', '-it', '-u', 'taxiway', '-e', 'HOME=/home/taxiway', runtime,
                     'bash', '-lc', 'unset ANTHROPIC_API_KEY ANTHROPIC_AUTH_TOKEN ANTHROPIC_BASE_URL CLAUDE_CODE_OAUTH_TOKEN; claude auth login']
        elif ref['driver'] == 'lima':
            login = ['limactl', 'shell', runtime, 'bash', '-lc',
                     'unset ANTHROPIC_API_KEY ANTHROPIC_AUTH_TOKEN ANTHROPIC_BASE_URL CLAUDE_CODE_OAUTH_TOKEN; claude auth login']
        else:
            raise RuntimeError('Unsupported lab driver')
        raise RuntimeError(f'Claude reference {lab} requires native login. Run: {shlex.join(login)}; then rerun the failed scenario to verify before provisioning')
    category = status.decode() if status in (b'setup', b'network', b'model', b'provider') else 'setup'
    raise RuntimeError(f'Claude reference {lab} preflight failed ({category}); authentication was not confirmed rejected; captured output withheld')


def _claude_onboarding(source):
    state = json.loads(guest(source, """python3 - <<'PY'
import json
from pathlib import Path
config = json.loads((Path.home() / '.claude.json').read_text())
print(json.dumps({key: config[key] for key in
                 ('hasCompletedOnboarding', 'lastOnboardingVersion') if key in config}))
PY"""))
    if state.get("hasCompletedOnboarding") is not True:
        raise RuntimeError("Complete Claude interactive onboarding in the reference lab first")
    if "lastOnboardingVersion" in state and not isinstance(state["lastOnboardingVersion"], str):
        raise RuntimeError("Reference lab has invalid Claude onboarding metadata")
    return state


def _write_claude_onboarding(target, state):
    guest(target, """python3 -c '
import json, os, pathlib, sys, tempfile
path = pathlib.Path.home() / ".claude.json"
config = json.loads(path.read_text()) if path.exists() else {}
config.update(json.load(sys.stdin))
fd, name = tempfile.mkstemp(prefix=".claude-onboarding-", dir=path.parent)
try:
    with os.fdopen(fd, "w") as output:
        json.dump(config, output, indent=2)
        output.write("\\n")
    os.chmod(name, 0o600)
    os.replace(name, path)
finally:
    if os.path.exists(name): os.unlink(name)
'""", data=json.dumps(state).encode())


def propagate_claude_onboarding(source, target):
    """Reuse completed setup flags, preserving target account/workspace settings."""
    if source == target:
        raise RuntimeError("Onboarding source and target must be different labs")
    _write_claude_onboarding(target, _claude_onboarding(source))


def propagate_claude_auth(source, target, *, overwrite=False):
    """Copy a reference login and completed onboarding; preserve lab-specific config."""
    if source == target:
        raise RuntimeError("Authentication source and target must be different labs")
    require_claude_auth(source)
    guest(target, 'command -v claude >/dev/null')
    present = guest(target, 'if [ -e "$HOME/.claude/.credentials.json" ]; then printf present; fi')
    if present and not overwrite:
        raise RuntimeError("Target already has Claude credentials; use overwrite explicitly")
    onboarding = _claude_onboarding(source)
    credential = guest(source, 'cat "$HOME/.claude/.credentials.json"')
    try:
        # Validate without ever including file contents in an error message.
        oauth = json.loads(credential).get("claudeAiOauth", {})
        if not isinstance(oauth, dict) or not oauth.get("accessToken") or not oauth.get("refreshToken"):
            raise ValueError()
    except (ValueError, AttributeError):
        raise RuntimeError("Reference lab has invalid Claude OAuth credentials") from None
    try:
        guest(target, '''set -euo pipefail
umask 077
install -d -m 0700 "$HOME/.claude"
tmp=$(mktemp "$HOME/.claude/.credentials.XXXXXX")
trap 'rm -f "$tmp"' EXIT
cat > "$tmp"
chmod 0600 "$tmp"
mv -f "$tmp" "$HOME/.claude/.credentials.json"
''', data=credential)
    finally:
        del credential
    _write_claude_onboarding(target, onboarding)


def run_in_lab(lab, argv, *, agent=None, timeout=180, workdir="/lab/work"):
    """Run argv with the lab's gateway environment; return captured stdout."""
    script = '''set -euo pipefail
set -a
source "$HOME/.config/taxiway/env"
set +a
'''
    if agent == "claude-code":
        script += '''source /lab/agents/claude-code/env.sh
claude_code_load_env
export ANTHROPIC_BASE_URL="${TAXIWAY_LITELLM_BASE_URL%/}"
export ANTHROPIC_CUSTOM_HEADERS="x-litellm-api-key: Bearer ${TAXIWAY_LITELLM_API_KEY}"
export CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1
'''
    elif agent not in (None, "codex"):
        raise RuntimeError("Unsupported agent environment")
    if not argv or timeout <= 0:
        raise RuntimeError("A command and positive timeout are required")
    script += f"exec timeout --kill-after=10s {timeout}s " + shlex.join(argv)
    return guest(lab, script, timeout=timeout + 30, workdir=workdir)


@contextmanager
def temporary_lab(orch, *, auth_lab=None, driver="docker", settings=None,
                  repo=None, taxiway="./taxiway", timeout=900):
    """Own one randomly named lab; preserve reference labs and shared runtime."""
    validate_context()
    if auth_lab:
        # A real native request (including native refresh) must pass before provisioning.
        require_claude_auth(auth_lab)
    if driver not in ("docker", "lima"):
        raise RuntimeError("Unsupported lab driver")
    name = "live-test-" + uuid.uuid4().hex[:12]
    if (Path(os.environ["TAXIWAY_LAB_STATE_DIR"]) / name).exists():
        raise RuntimeError("Temporary lab name already exists")
    argv = [taxiway, "up", name, "--driver", driver, "--type", orch, "--skip-auth-check"]
    for key, value in (settings or {}).items():
        argv += ["--set", f"{key}={value}"]
    if repo:
        argv += ["--repo", repo]
    if auth_lab:
        argv += ["--prepare-only"]
    try:
        command(argv, timeout=timeout)
        if auth_lab:
            propagate_claude_auth(auth_lab, name)
            command([taxiway, "run", name, "--skip-auth-check"], timeout=timeout)
        yield name
    finally:
        scenario_failed = sys.exc_info()[0] is not None
        # No destroy, prune, or reference-lab removal. Also clean partial setup.
        try:
            command([taxiway, "rm", name, "--yes"], timeout=180)
        except (RuntimeError, OSError, subprocess.TimeoutExpired):
            message = f"Cleanup failed for {name}; remove that lab with taxiway rm --yes"
            if scenario_failed:
                print(message, file=sys.stderr)
            else:
                raise RuntimeError(message) from None


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    actions = parser.add_subparsers(dest="action", required=True)
    preflight = actions.add_parser("preflight", help="Verify a reference with a bounded native Claude request")
    preflight.add_argument("--source", required=True)
    auth = actions.add_parser("auth", help="Propagate an existing Claude lab login")
    auth.add_argument("--source", required=True)
    auth.add_argument("--target", required=True)
    auth.add_argument("--overwrite", action="store_true")
    run = actions.add_parser("run", help="Run a smoke command in an owned temporary lab")
    run.add_argument("--type", required=True)
    run.add_argument("--auth-lab")
    run.add_argument("--driver", choices=("docker", "lima"), default="docker")
    run.add_argument("--agent", choices=("claude-code", "codex"))
    run.add_argument("--set", action="append", default=[])
    run.add_argument("--repo")
    run.add_argument("--taxiway", default="./taxiway")
    run.add_argument("--timeout", type=int, default=180)
    run.add_argument("--expect", help="Required stdout marker; captured output is not printed")
    run.add_argument("command", nargs=argparse.REMAINDER)
    args = parser.parse_args()
    validate_context()
    if args.action == "preflight":
        require_claude_auth(args.source)
        print("PASS: native Claude reference request completed; no scenario lab created.")
        return
    if args.action == "auth":
        propagate_claude_auth(args.source, args.target, overwrite=args.overwrite)
        print("Claude authentication propagated; restart the target session with taxiway start.")
        return
    argv = args.command[1:] if args.command[:1] == ["--"] else args.command
    if not argv:
        parser.error("Pass the guest command after --")
    settings = {}
    for setting in args.set:
        key, sep, value = setting.partition("=")
        if not sep or not key:
            parser.error("Settings must be key=value")
        settings[key] = value
    with temporary_lab(args.type, auth_lab=args.auth_lab, driver=args.driver,
                       settings=settings, repo=args.repo, taxiway=args.taxiway) as lab:
        output = run_in_lab(lab, argv, agent=args.agent, timeout=args.timeout)
        if args.expect and args.expect.encode() not in output:
            raise RuntimeError("Expected stdout marker missing")
    print("PASS: command completed; temporary lab removed; reference login preserved.")


if __name__ == "__main__":
    try:
        main()
    except subprocess.TimeoutExpired:
        print("FAIL: timeout (captured output withheld)", file=sys.stderr)
        sys.exit(1)
    except RuntimeError as error:
        print(f"FAIL: {error} (captured output withheld)", file=sys.stderr)
        sys.exit(1)
    except OSError:
        print("FAIL: setup, execution, or cleanup failed (captured output withheld)", file=sys.stderr)
        sys.exit(1)
