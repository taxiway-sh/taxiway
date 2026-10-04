"""Real recording lifecycle and owned cleanup, without provider authentication."""

import argparse
import json
import os
from pathlib import Path
import shlex
import subprocess
import sys
import time

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


def record(lab, taxiway):
    guest(lab, "tmux new-session -d -s claude-code 'bash --noprofile --norc'; tmux set-option -g prefix C-a")
    command([taxiway, "record", "start", lab, "--name", "live-lifecycle"], timeout=60)
    recording = session(lab)
    recorder = shlex.quote("=" + recording["recorder_session"])
    cast = Path(recording["cast_path_host"])
    # Require the real recording client before sending fixture output.
    def attached():
        tty = guest(lab, f"tmux show-option -qv -t {recorder} @taxiway-recorder-client", timeout=10).strip()
        clients = guest(lab, "tmux list-clients -F '#{client_tty}'", timeout=10).splitlines()
        return bool(tty) and tty in clients
    wait_for(attached)
    guest(lab, "tmux send-keys -t '=claude-code' 'printf LIVE_RECORDING_PROOF' Enter", timeout=10)
    wait_for(lambda: "LIVE_RECORDING_PROOF" in cast_output(cast))
    command([taxiway, "record", "stop", lab], timeout=60)
    require(session(lab)["state"] == "stopped", "Recording index was not stopped")
    guest(lab, f"if tmux has-session -t {recorder}; then exit 1; fi; tmux has-session -t '=claude-code'", timeout=10)
    require("LIVE_RECORDING_PROOF" in cast_output(cast), "Stopped cast lost captured output")
    command([taxiway, "record", "rm", lab, "--id", recording["id"]], timeout=60)
    require(not cast.exists(), "Recording removal left its cast")

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
    outcomes = ("success", "failure", "timeout") if args.outcome == "all" else (args.outcome,)
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
                        record(lab, args.taxiway)
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
    except (RuntimeError, OSError, subprocess.TimeoutExpired):
        print(f"FAIL {prefix} phase={phase}; captured output withheld; inspect scoped lab state for partial setup", file=sys.stderr)
        raise


if __name__ == "__main__":
    try:
        sys.exit(main() or 0)
    except (RuntimeError, OSError, subprocess.TimeoutExpired):
        sys.exit(1)
