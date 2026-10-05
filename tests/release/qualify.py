#!/usr/bin/env python3
"""Local evidence ledger for the release recipe; never publishes anything."""
import argparse
import fcntl
from datetime import datetime, timezone
import json
import os
from pathlib import Path
import platform
import re
import shlex
import signal
import subprocess
import sys
import uuid

CHECKS = {
    "pre": ("core", "scripts", "site", "gateway", "e2e-codex", "e2e-claude-code",
            "e2e-gastown", "live-codex", "live-claude", "live-gastown", "lima",
            "recording", "docs-cli", "package", "isolation-cleanup", "beta-compat", "breaking-changes", "review"),
    "post": ("published-assets", "install-matrix", "mac-install", "installed-smoke"),
}
SKIP = re.compile(r"--- SKIP:|\bSKIP(?:PED)?\b|\bskipped[=: ]+[1-9]", re.IGNORECASE)


def git(*args):
    return subprocess.check_output(["git", *args], text=True).strip()


def candidate_check(data):
    if data["phase"] == "post" and git("rev-parse", f"refs/tags/{data['ref']}^{{commit}}") != data["candidate"]:
        raise ValueError("Published tag moved: start a new report and investigate the release.")
    if git("rev-parse", "HEAD") != data["candidate"]:
        raise ValueError("Candidate changed: start a new report; old results do not qualify this HEAD.")
    if git("status", "--porcelain"):
        raise ValueError("Dirty checkout: commit the candidate before qualification.")


def save(folder, data):
    target = folder / "report.json"
    temporary = folder / "report.json.tmp"
    temporary.write_text(json.dumps(data, indent=2) + "\n")
    temporary.replace(target)
    lines = ["# Taxiway qualification evidence", "", f"Candidate: `{data['candidate']}`",
             f"Phase: {data['phase']}; platform: {data['platform']}; driver: {data['driver']}",
             f"Checkout: `{data['checkout']}`; initialized clean: yes", "",
             "This ledger is not publication authorization or automatic certification.", ""]
    for name, item in data["checks"].items():
        lines += [f"## {name}: {item['status']}", "", item.get("note", "No evidence recorded."),
                  f"Proof: {item.get('evidence', 'none')}",
                  f"Command: `{item.get('command', 'manual / not executed')}`", ""]
    (folder / "report.md").write_text("\n".join(lines))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--report", type=Path, required=True, help="Private directory outside checkout")
    commands = parser.add_subparsers(dest="action", required=True)
    init = commands.add_parser("init")
    init.add_argument("--candidate", required=True, help="Exact source commit or published tag")
    init.add_argument("--phase", choices=CHECKS, default="pre")
    init.add_argument("--driver", default="Docker; Lima not yet qualified")
    run = commands.add_parser("run", help="Run an existing check without a shell")
    run.add_argument("check")
    run.add_argument("--timeout", type=float, default=1800)
    run.add_argument("command", nargs=argparse.REMAINDER)
    record = commands.add_parser("record", help="Record independently inspected evidence")
    record.add_argument("check")
    record.add_argument("--candidate", required=True)
    record.add_argument("--status", choices=("PASS", "FAIL", "NOT_EXECUTED"), required=True)
    record.add_argument("--evidence", type=Path)
    record.add_argument("--note", required=True, help="Sanitized observed results, limits and pending action")
    commands.add_parser("report")
    args = parser.parse_args()
    folder = args.report.expanduser().resolve()
    checkout = Path(git("rev-parse", "--show-toplevel")).resolve()
    if folder == checkout or checkout in folder.parents:
        raise ValueError("Evidence must be outside the checkout (including ignored directories).")
    os.umask(0o077)
    if args.action == "init":
        if folder.exists():
            raise ValueError("Report already exists: resume it or choose a new directory.")
        if args.phase == "post" and not re.fullmatch(r"v\d+\.\d+\.\d+(?:[-+][\w.-]+)?", args.candidate):
            raise ValueError("Post-publication qualification requires an explicit version tag, not latest/SHA.")
        ref = f"refs/tags/{args.candidate}" if args.phase == "post" else args.candidate
        sha = git("rev-parse", f"{ref}^{{commit}}")
        data = {"candidate": sha, "phase": args.phase, "checkout": str(checkout),
                "platform": f"{platform.system()}/{platform.machine()}", "driver": args.driver,
                "created": datetime.now(timezone.utc).isoformat(),
                "ref": args.candidate,
                "checks": {name: {"status": "NOT_EXECUTED"} for name in CHECKS[args.phase]}}
        candidate_check(data)
        folder.mkdir(parents=True, mode=0o700)
        save(folder, data)
        print(f"Initialized {args.phase} evidence for {sha}: {folder / 'report.md'}")
        return 0
    lock = (folder / "active.lock").open("a")
    try:
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
    except BlockingIOError:
        raise ValueError("Another command is active for this report; inspect its process and proof.")
    data = json.loads((folder / "report.json").read_text())
    candidate_check(data)
    if args.action == "report":
        save(folder, data)
        for name, item in data["checks"].items():
            print(f"{name}: {item['status']}")
        print("Review evidence and coverage limits before deciding release readiness.")
        return int(any(item["status"] != "PASS" for item in data["checks"].values()))
    if args.check not in data["checks"]:
        raise ValueError("Unknown check for this phase; see report.")
    if args.action == "record":
        if git("rev-parse", f"{args.candidate}^{{commit}}") != data["candidate"]:
            raise ValueError("Evidence candidate does not match report.")
        if not args.note.strip():
            raise ValueError("Describe observed results and coverage limits.")
        if args.status != "NOT_EXECUTED" and (not args.evidence or not args.evidence.is_file()):
            raise ValueError("PASS/FAIL requires an existing sanitized proof file.")
        item = {"status": args.status, "note": args.note,
                "evidence": str(args.evidence.resolve()) if args.evidence else "none",
                "kind": "independently inspected; not automatically verified"}
    else:
        command = args.command
        # argparse REMAINDER keeps run options placed after the check name.
        if command[:1] == ["--timeout"]:
            args.timeout = float(command[1])
            command = command[2:]
        if command[:1] == ["--"]:
            command = command[1:]
        if not command or args.timeout <= 0:
            raise ValueError("Supply a command and positive timeout.")
        proof = folder / f"{args.check}-{uuid.uuid4().hex[:12]}.log"
        item = {"status": "NOT_EXECUTED", "command": shlex.join(command), "evidence": str(proof),
                "note": "Interrupted/in progress; reconcile processes and resources before retrying."}
        data["checks"][args.check] = item
        save(folder, data)
        with proof.open("wb") as output:
            process = subprocess.Popen(command, stdout=output, stderr=subprocess.STDOUT, start_new_session=True)
            item["pid"] = process.pid
            save(folder, data)
            try:
                code = process.wait(timeout=args.timeout)
            except (subprocess.TimeoutExpired, KeyboardInterrupt):
                os.killpg(process.pid, signal.SIGTERM)
                try:
                    process.wait(timeout=5)
                except subprocess.TimeoutExpired:
                    pass
                # The parent may exit on TERM while a child ignores it.
                try:
                    os.killpg(process.pid, signal.SIGKILL)
                except ProcessLookupError:
                    pass
                process.wait()
                code = -1
                item["note"] = "Timeout/interruption: FAIL; inspect and clean owned resources before retry."
        text = proof.read_text(errors="replace")
        item["status"] = "FAIL" if code else ("NOT_EXECUTED" if SKIP.search(text) else "PASS")
        if code != -1:
            item["note"] = f"Exit {code}. " + ("Skip detected; inspect coverage." if item["status"] == "NOT_EXECUTED" else "Inspect proof and actual test counts; exit zero alone cannot prove coverage.")
        item["exit_code"] = code
        try:
            candidate_check(data)
        except ValueError as error:
            item.update(status="FAIL", note=str(error))
    item["time"] = datetime.now(timezone.utc).isoformat()
    data["checks"][args.check] = item
    save(folder, data)
    print(f"{args.check}: {item['status']}; proof: {item['evidence']}")
    return int(item["status"] != "PASS")


if __name__ == "__main__":
    try:
        sys.exit(main())
    except (ValueError, OSError, subprocess.CalledProcessError) as error:
        print(f"Qualification error: {error}", file=sys.stderr)
        sys.exit(2)
