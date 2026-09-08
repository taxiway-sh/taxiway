"""Check the real Gas Town sessions without repairing or launching agents."""

import json
from pathlib import Path
import re
import subprocess


town = Path("/lab/work/gt")


def run(args):
    return subprocess.check_output(args, cwd=town, text=True, timeout=60)


status = json.loads(run(["gt", "status", "--json"]))
socket = status["tmux"]["socket"]
sessions = set(run(["tmux", "-L", socket, "list-sessions", "-F", "#{session_name}"]).splitlines())
doctor = subprocess.run(
    ["gt", "doctor"], cwd=town, text=True, stdout=subprocess.PIPE,
    stderr=subprocess.STDOUT, timeout=60,
)

# Ignore unrelated doctor warnings, but never accept a startup zombie cleanup
# merely because its final report says the problem was fixed.
for phase, output in (
    ("startup", (town / ".runtime/doctor-fix.log").read_text()),
    ("doctor", doctor.stdout),
):
    check = "\n".join(line for line in output.splitlines() if "zombie-sessions" in line)
    print(f"{phase}: {check}", flush=True)
    assert not re.search(r"Found [1-9][0-9]* zombie session", check), f"{phase}: zombie sessions detected"
    assert (
        "No zombie sessions found" in check
        or re.search(r"All [1-9][0-9]* Gas Town sessions have running Claude processes", check)
    ), f"{phase}: missing or unsuccessful zombie check"

agents = list(status.get("agents") or [])
for rig in status.get("rigs") or []:
    agents.extend(rig.get("agents") or [])

checked = 0
for agent in agents:
    # Boot and dogs may complete normally between observations. An absent idle
    # agent is not a failure: this check concerns the persistent sessions present.
    if agent["session"] not in sessions or agent["role"] in ("boot", "dog"):
        continue
    checked += 1
    assert agent["running"], f"Present session {agent['session']} is not recognized as running by Gas Town"
    print(f"{agent['session']}: recognized as running", flush=True)

assert checked, "No persistent agent session was checked"
