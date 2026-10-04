"""Qualify Lima creation and readiness retry on one owned, provider-free VM."""

import argparse
import os
from pathlib import Path
import subprocess
import sys

from taxiway_live import command, guest, runtime_id, temporary_lab, validate_context


def instances():
    output = command(["limactl", "list", "--format={{.Name}} {{.Status}}"], timeout=15)
    return dict(line.split() for line in output.decode().splitlines() if line.strip())


def require(condition, message):
    if not condition:
        raise RuntimeError(message)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--driver", required=True, choices=("lima",))
    parser.add_argument("--taxiway", default="./taxiway")
    parser.add_argument("--setup-timeout", type=int, default=300)
    args = parser.parse_args()
    require(args.setup_timeout > 0, "Setup timeout must be positive")
    validate_context()
    revision = command(["git", "rev-parse", "HEAD"], timeout=10).decode().strip()
    if subprocess.run(["git", "diff", "--quiet", "HEAD"], timeout=10).returncode:
        revision += "+dirty"
    preserved = instances()
    os.environ["TAXIWAY_LIMA_START_TIMEOUT"] = f"{args.setup_timeout}s"
    lab = runtime = None
    phase = "setup"
    failed = False
    try:
        with temporary_lab("claude-code", driver=args.driver, taxiway=args.taxiway,
                           timeout=args.setup_timeout, prepare_only=True) as lab:
            runtime = runtime_id(lab)
            phase = "readiness"
            require(guest(lab, "test -s /run/lima-boot-done && test -w /lab/work && printf READY",
                          timeout=15) == b"READY", "Fresh guest is not ready")
            print("PASS fresh-create guest-execution-and-boot-readiness", flush=True)
            marker = Path(os.environ["TAXIWAY_LAB_STATE_DIR"]) / lab / "phases/create.done"
            marker.unlink()
            phase = "partial-running-retry"
            guest(lab, "sudo mv /run/lima-boot-done /run/lima-boot-done.taxiway-test", timeout=15)
            try:
                require(instances().get(runtime) == "Running", "Fixture must remain Running")
                result = subprocess.run([args.taxiway, "create", lab, "--driver", "lima",
                                         "--type", "claude-code"], stdout=subprocess.PIPE,
                                        stderr=subprocess.PIPE, timeout=30)
                require(result.returncode != 0, "Running but unfinished guest was accepted")
                require(b"Lima boot scripts have not finished" in result.stderr,
                        "Retry did not identify unfinished boot scripts")
                require(not marker.exists(), "Failed readiness retry marked create complete")
                print("PASS partial-running-retry rejects-unfinished-boot", flush=True)
            finally:
                guest(lab, "sudo mv /run/lima-boot-done.taxiway-test /run/lima-boot-done", timeout=15)
            phase = "ready-retry"
            command([args.taxiway, "create", lab, "--driver", "lima", "--type", "claude-code"], timeout=30)
            require(marker.exists(), "Successful readiness retry did not complete create")
            print("PASS ready-retry preserves-and-reuses-owned-vm", flush=True)
            phase = "restart"
            command([args.taxiway, "down", lab], timeout=120)
            require(instances().get(runtime) == "Stopped", "Owned VM did not stop")
            command([args.taxiway, "create", lab, "--driver", "lima", "--type", "claude-code"],
                    timeout=args.setup_timeout)
            require(guest(lab, "test -s /run/lima-boot-done && printf READY", timeout=15) == b"READY",
                    "Restarted guest is not ready")
            print("PASS restart guest-execution-and-boot-readiness", flush=True)
    except (RuntimeError, OSError, subprocess.TimeoutExpired):
        failed = True
        print(f"FAIL driver=lima revision={revision} phase={phase}; captured output withheld", file=sys.stderr)
    finally:
        try:
            after = instances()
            require(after == preserved, "Cleanup changed unrelated VMs or left an owned VM")
            if lab:
                require(not (Path(os.environ["TAXIWAY_LAB_STATE_DIR"]) / lab).exists(),
                        "Cleanup left owned lab state")
            print(f"PASS driver=lima revision={revision} cleanup=verified", flush=True)
        except (RuntimeError, OSError, subprocess.TimeoutExpired):
            failed = True
            print("FAIL owned cleanup or unrelated VM preservation; inspect scoped state", file=sys.stderr)
    return int(failed)


if __name__ == "__main__":
    raise SystemExit(main())
