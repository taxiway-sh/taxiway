"""Native Lima recording and Docker/Lima exceptional owned cleanup E2Es."""

import argparse
import json
import os
from pathlib import Path
import shlex
import subprocess
import sys
import tempfile
import time

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "live"))

from taxiway_live import command, guest, lab_ref, runtime_id, temporary_lab, validate_context


def require(condition, message):
    if not condition:
        raise RuntimeError(message)


def wait_for(check, *, timeout=30):
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        if check():
            return
        time.sleep(0.25)
    raise RuntimeError("Behavior assertion timed out; captured output withheld")


def ready(lab):
    # Exercise SSH/exec, dependencies and both mount directions, not VM status.
    output = guest(lab, '''set -euo pipefail
test -d /lab/work && test -r /lab/infra/commands/bootstrap.sh
for tool in python3 tmux asciinema timeout; do command -v "$tool" >/dev/null; done
test -w /lab/recordings
printf fixture > /lab/recordings/live-readiness
printf READY
''', timeout=30)
    require(output == b"READY", "Guest readiness assertion failed")
    host = Path(os.environ["TAXIWAY_LAB_STATE_DIR"]) / lab / "recordings/live-readiness"
    require(host.read_bytes() == b"fixture", "Guest recording mount is not host-visible")
    host.unlink()


def session(lab):
    path = Path(os.environ["TAXIWAY_LAB_STATE_DIR"]) / lab / "recordings/recordings.json"
    return json.loads(path.read_text())["sessions"][-1]


def cast_output(path):
    if not path.exists():
        return ""
    lines = path.read_text().splitlines()
    if not lines:
        return ""
    require(json.loads(lines[0]).get("version") == 2, "Invalid asciicast header")
    output = []
    for line in lines[1:]:
        try:
            event = json.loads(line)
        except json.JSONDecodeError:
            continue  # The live writer can expose its current partial line.
        if len(event) == 3 and event[1] == "o":
            output.append(event[2])
    return "".join(output)


def recorder_attached(lab, recorder_option_target):
    try:
        tty = guest(lab, f"tmux show-option -qv -t {recorder_option_target} @taxiway-recorder-client", timeout=10).strip()
        clients = guest(lab, "tmux list-clients -F '#{client_tty}'", timeout=10).splitlines()
        return bool(tty) and tty in clients
    except RuntimeError:
        # record start launches its client asynchronously; poll within the bound.
        return False


def recording_containment(lab, taxiway, recording):
    """A real guest modifies its mounted index; outside fixtures stay private."""
    index = Path(os.environ["TAXIWAY_LAB_STATE_DIR"]) / lab / "recordings/recordings.json"
    original = index.read_bytes()
    with tempfile.TemporaryDirectory(prefix="taxiway-recording-containment-") as directory:
        outside = Path(directory) / "sentinel.cast"
        outside.write_bytes(b"harmless outside sentinel")
        try:
            for mode in ("outside", "symlink"):
                script = f'''python3 - <<'PY'
import json, pathlib
p = pathlib.Path('/lab/recordings/recordings.json')
data = json.loads({original.decode()!r})
entry = data['sessions'][-1]
outside = {str(outside)!r}
if {mode!r} == 'symlink':
    link = pathlib.Path('/lab/recordings/containment.cast')
    link.symlink_to(outside)
    entry['cast_path_host'] = {str(index.parent / 'containment.cast')!r}
    entry['cast_path'] = '/lab/recordings/containment.cast'
else:
    entry['cast_path_host'] = outside
p.write_text(json.dumps(data))
PY'''
                guest(lab, script, timeout=15)
                poisoned = index.read_bytes()
                for args in (("record", "list", lab),
                             ("record", "rm", lab, "--id", recording["id"]),
                             ("record", "analyze", lab, "--prompt-only")):
                    rejected = False
                    try:
                        command([taxiway, *args], timeout=30)
                    except RuntimeError:
                        rejected = True
                    require(rejected, "Untrusted recording metadata was accepted")
                    require(outside.read_bytes() == b"harmless outside sentinel", "Outside recording sentinel changed")
                    require(index.read_bytes() == poisoned, "Rejected recording command modified its index")
                guest(lab, "rm -f /lab/recordings/containment.cast", timeout=15)
        finally:
            # Restore only this owned test lab, even after a failed assertion.
            index.write_bytes(original)
            guest(lab, "rm -f /lab/recordings/containment.cast", timeout=15)
    prompt = command([taxiway, "record", "analyze", lab, "--prompt-only"], timeout=30)
    require(Path(recording["cast_path_host"]).read_bytes() in prompt,
            "Valid analysis lost captured artifact contents")

def record(lab, taxiway):
    print("STEP recording target-session", flush=True)
    guest(lab, "tmux new-session -d -s claude-code 'bash --noprofile --norc'; tmux set-option -g prefix C-a")
    print("STEP recording start", flush=True)
    command([taxiway, "record", "start", lab, "--name", "live-lifecycle"], timeout=60)
    recording = session(lab)
    recorder = shlex.quote("=" + recording["recorder_session"])
    recorder_option_target = shlex.quote(recording["recorder_session"])
    cast = Path(recording["cast_path_host"])
    # Require the real recording client before sending fixture output.
    print("STEP recording live-client", flush=True)
    wait_for(lambda: recorder_attached(lab, recorder_option_target))
    guest(lab, "tmux send-keys -t claude-code 'printf LIVE_RECORDING_PROOF' Enter", timeout=10)
    print("STEP recording capture", flush=True)
    wait_for(lambda: "LIVE_RECORDING_PROOF" in cast_output(cast))
    print("STEP recording stop", flush=True)
    command([taxiway, "record", "stop", lab], timeout=60)
    require(session(lab)["state"] == "stopped", "Recording index was not stopped")
    guest(lab, f"if tmux has-session -t {recorder}; then exit 1; fi; tmux has-session -t '=claude-code'", timeout=10)
    require("LIVE_RECORDING_PROOF" in cast_output(cast), "Stopped cast lost captured output")
    print("STEP recording host-containment", flush=True)
    recording_containment(lab, taxiway, recording)
    print("STEP recording remove", flush=True)
    command([taxiway, "record", "rm", lab, "--id", recording["id"]], timeout=60)
    require(not cast.exists(), "Recording removal left its cast")

    print("STEP recording stopped-lab-recovery", flush=True)
    command([taxiway, "record", "start", lab, "--name", "stopped-recovery"], timeout=60)
    recovering = session(lab)
    recovering_cast = Path(recovering["cast_path_host"])
    wait_for(lambda: recovering_cast.exists() and recovering_cast.stat().st_size > 0)
    command([taxiway, "down", lab], timeout=120)
    command([taxiway, "record", "stop", lab], timeout=30)
    require(session(lab)["state"] == "stopped", "Stopped-lab recovery left an active index")
    require(recovering_cast.exists(), "Stopped-lab recovery removed its cast")
    command([taxiway, "record", "rm", lab, "--id", recovering["id"]], timeout=30)
    require(not recovering_cast.exists(), "Stopped-lab recording removal left its cast")


def recording_diagnostics(lab):
    """Only fixture protocol/state metadata; never cast text or environment values."""
    data = {"index_present": False}
    path = Path(os.environ["TAXIWAY_LAB_STATE_DIR"]) / lab / "recordings/recordings.json"
    if path.exists():
        entries = json.loads(path.read_text()).get("sessions", [])
        data["index_present"] = True
        data["entries"] = len(entries)
        if entries:
            entry = entries[-1]
            data["recording_state"] = entry.get("state")
            cast = Path(entry["cast_path_host"])
            data["cast_exists"] = cast.exists()
            if cast.exists():
                data["cast_bytes"] = cast.stat().st_size
                lines = cast.read_text().splitlines()
                if lines:
                    header = json.loads(lines[0])
                    data["cast_version"] = header.get("version")
    try:
        data["guest"] = json.loads(guest(lab, '''python3 - <<'PY'
import json, subprocess
def call(argv):
    result = subprocess.run(argv, capture_output=True, timeout=5)
    return result.returncode, result.stdout
code, output = call(['tmux', 'list-sessions', '-F', '#{session_name}'])
sessions = output.decode().splitlines() if code == 0 else []
code, output = call(['tmux', 'list-clients', '-F', '#{client_tty}'])
print(json.dumps({'tmux_target': 'claude-code' in sessions,
                  'recorder_count': sum(name.startswith('taxiway-record-') for name in sessions),
                  'client_count': len(output.splitlines()) if code == 0 else 0}))
PY''', timeout=15))
    except (RuntimeError, OSError, subprocess.TimeoutExpired):
        data["guest"] = "diagnostic-unavailable"
    print("DIAGNOSTIC " + json.dumps(data), flush=True)


def instances(driver):
    if driver == "lima":
        return command(["limactl", "list", "--format={{.Name}} {{.Status}}"], timeout=30).splitlines()
    return command(["docker", "ps", "-a", "--format={{.Names}} {{.Status}}"], timeout=30).splitlines()


def cleanup_verified(driver, lab, runtime):
    require(not (Path(os.environ["TAXIWAY_LAB_STATE_DIR"]) / lab).exists(), "Temporary lab state survived cleanup")
    require(not any(line.split()[0] == runtime.encode() for line in instances(driver) if line.split()),
            "Temporary guest survived cleanup")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--driver", required=True, choices=("docker", "lima"))
    parser.add_argument("--taxiway", default="./taxiway")
    parser.add_argument("--outcome", choices=("success", "failure", "timeout", "all"), default="all")
    parser.add_argument("--setup-timeout", type=int, default=900)
    args = parser.parse_args()
    validate_context()
    require(args.setup_timeout > 0, "Setup timeout must be positive")
    revision = command(["git", "rev-parse", "HEAD"], timeout=10).decode().strip()
    if command(["git", "status", "--porcelain", "--untracked-files=no"], timeout=10):
        revision += "+dirty"
    prefix = f"driver={args.driver} revision={revision}"
    try:
        initial = instances(args.driver)
    except (RuntimeError, OSError, subprocess.TimeoutExpired):
        print(f"NOT_EXECUTED {prefix} prerequisite=driver-inventory-unavailable; captured output withheld", file=sys.stderr)
        return 2
    # Preserve unrelated/reference guests including their current status.
    preserved = {line.split()[0]: line.split()[1:] for line in initial if line.split()}
    # Docker recording is covered by the existing Go orchestrator E2Es.
    require(args.driver != "docker" or args.outcome != "success",
            "Use the Go orchestrator E2Es for Docker recording")
    outcomes = (("success", "failure", "timeout") if args.driver == "lima"
                else ("failure", "timeout")) if args.outcome == "all" else (args.outcome,)
    phase = "setup"
    try:
        for outcome in outcomes:
            lab = runtime = None
            phase = "setup"
            expected_failure = False
            try:
                with temporary_lab("claude-code", driver=args.driver, taxiway=args.taxiway,
                                   timeout=args.setup_timeout, prepare_only=True) as lab:
                    runtime = runtime_id(lab)
                    require(lab_ref(lab)["driver"] == args.driver, "Persisted driver does not match selection")
                    phase = "guest-readiness"
                    ready(lab)
                    phase = outcome
                    if outcome == "success":
                        try:
                            record(lab, args.taxiway)
                        except (RuntimeError, OSError, subprocess.TimeoutExpired):
                            recording_diagnostics(lab)
                            raise
                    elif outcome == "failure":
                        guest(lab, "exit 7", timeout=10)
                    else:
                        guest(lab, "exec sleep 30", timeout=1)
            except RuntimeError as error:
                if phase == "failure" and str(error).endswith("exited 7"):
                    expected_failure = True
                else:
                    raise
            except subprocess.TimeoutExpired:
                if phase == "timeout":
                    expected_failure = True
                else:
                    raise
            require(outcome == "success" or expected_failure, "Expected failure/timeout did not occur")
            phase = "cleanup-verification"
            cleanup_verified(args.driver, lab, runtime)
            current = {line.split()[0]: line.split()[1:] for line in instances(args.driver) if line.split()}
            # Docker's human duration changes; Lima's status is stable and actionable.
            for name, status in preserved.items():
                require(name in current, "Unrelated guest was removed")
                if args.driver == "lima":
                    require(current[name] == status, "Unrelated Lima guest status changed")
            print(f"PASS {prefix} outcome={outcome} guest-readiness=verified cleanup=verified", flush=True)
    except (RuntimeError, OSError, subprocess.TimeoutExpired) as error:
        # Helper RuntimeErrors are sanitized; never print OS paths or captured output.
        reason = str(error) if isinstance(error, RuntimeError) else type(error).__name__
        print(f"FAIL {prefix} phase={phase} reason={reason}; captured output withheld; inspect scoped lab state for partial setup", file=sys.stderr)
        raise


if __name__ == "__main__":
    try:
        sys.exit(main() or 0)
    except (RuntimeError, OSError, subprocess.TimeoutExpired):
        sys.exit(1)
