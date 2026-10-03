"""Opt-in Gas Town lifecycle, handoff and model delegation in a real lab."""

import argparse
import json
import shlex
import subprocess
import sys
import time

from taxiway_live import command, guest, temporary_lab, validate_context


def status(lab):
    return json.loads(guest(lab, 'export PATH="$HOME/.local/bin:$PATH"; cd /lab/work/gt; gt status --json'))


def sessions(lab):
    snapshot = status(lab)
    socket = snapshot["tmux"]["socket"]
    names = guest(lab, f"tmux -L {shlex.quote(socket)} list-sessions -F '#{{session_name}}'").decode().splitlines()
    agents = list(snapshot.get("agents", []))
    for rig in snapshot.get("rigs", []):
        agents.extend(rig.get("agents", []))
    persistent = [a for a in agents if a.get("session") in names and a.get("role") not in ("boot", "dog")]
    assert persistent, "No persistent Gastown roles found"
    assert all(a.get("running") for a in persistent), "Gastown reports a non-running persistent role"
    assert "hq-mayor" in names, "Mayor tmux session missing"
    # Compare process configuration inside the guest: never return headers or keys.
    output = guest(lab, "python3 - " + shlex.quote(socket) + " <<'PY'\n" + r"""
import json, os, pathlib, subprocess, sys
root=pathlib.Path('/lab/work/gt')
onboarding=json.loads((pathlib.Path.home()/'.claude.json').read_text())
assert onboarding.get('hasCompletedOnboarding') is True, 'Claude interactive onboarding was not propagated'
auth=json.loads(subprocess.check_output(['claude','auth','status'],text=True))
assert auth.get('loggedIn') is True, 'Claude does not report a logged-in account'
agent=json.loads((root/'settings/agents.json').read_text())['agents']['claude-code-litellm']
model=agent['args'][agent['args'].index('--model')+1]
names=subprocess.check_output(['tmux','-L',sys.argv[1],'list-sessions','-F','#{session_name}'],text=True).splitlines()
checked=0
for name in names:
    if name not in ('hq-mayor','hq-deacon') and not name.endswith(('-refinery','-witness')) and '-crew-' not in name:
        continue
    pid=subprocess.check_output(['tmux','-L',sys.argv[1],'display-message','-p','-t',name,'#{pane_pid}'],text=True).strip()
    proc=pathlib.Path('/proc')/pid
    args=(proc/'cmdline').read_bytes().split(b'\0')
    env=dict(v.split(b'=',1) for v in (proc/'environ').read_bytes().split(b'\0') if b'=' in v)
    assert b'--dangerously-skip-permissions' in args, 'role lost mandatory bypass permissions'
    assert b'{"skipDangerousModePermissionPrompt":true}' in args, 'role lost bypass startup warning suppression'
    assert b'--model' in args, 'role lost model argument'
    assert args[args.index(b'--model')+1]==model.encode(), 'role model changed'
    for key in ('ANTHROPIC_BASE_URL','ANTHROPIC_CUSTOM_HEADERS'):
        assert env.get(key.encode())==agent['env'][key].encode(), 'role lost gateway routing'
    for alias in ('OPUS','SONNET','HAIKU'):
        assert env.get(('ANTHROPIC_DEFAULT_'+alias+'_MODEL').encode()), 'role lost alias mapping'
    assert env.get(b'TAXIWAY_CLAUDE_AVAILABLE_MODELS'), 'role lost managed model catalog'
    pane=subprocess.check_output(['tmux','-L',sys.argv[1],'capture-pane','-p','-t',name],text=True).lower()
    assert not any(prompt in pane for prompt in ('choose the text style','choose a theme','select login method','select the login method')), 'role is blocked on interactive onboarding'
    checked+=1
assert checked>=2, 'fewer than two persistent Claude roles checked'
print(json.dumps({'model':model,'checked':checked}))
""" + "\nPY", timeout=90)
    return socket, json.loads(output)


def deacon_patrol(lab):
    # Return only behavioral evidence, never transcript text or tool payloads.
    return json.loads(guest(lab, "python3 - <<'PY'\n" + r"""
import json, pathlib
root=pathlib.Path('/lab/work/gt')
heartbeat=root/'deacon/heartbeat.json'
hb=json.loads(heartbeat.read_text()) if heartbeat.exists() else {}
completed=set(); check_count=0; reports=0; interrupted=False
for path in (pathlib.Path.home()/'.claude/projects').rglob('*.jsonl'):
    calls={}
    for line in path.read_text(errors='replace').splitlines():
        try: event=json.loads(line)
        except ValueError: continue
        if event.get('cwd') != str(root/'deacon'): continue
        blocks=event.get('message',{}).get('content',[])
        if not isinstance(blocks,list): continue
        for block in blocks:
            if not isinstance(block,dict): continue
            if block.get('type')=='tool_use' and block.get('name')=='Bash':
                calls[block['id']]=block.get('input',{}).get('command','')
            elif block.get('type')=='tool_result':
                command=calls.get(block.get('tool_use_id'),'')
                output=str(block.get('content','')).lower()
                if 'gt sling mol-deacon-patrol deacon' in command:
                    interrupted |= bool(block.get('is_error')) or 'interrupt' in output
                if block.get('is_error'): continue
                for check in ('gt mail inbox','gt convoy','gt witness status',
                              'gt refinery status','gt deacon cleanup-orphans',
                              'gt dog','gt dispatch'):
                    if check in command:
                        completed.add(check)
                        check_count+=1
                if ('gt patrol report' in command and 'closed patrol ' in output
                        and 'new patrol:' in output):
                    reports+=1
print(json.dumps({'cycle':hb.get('cycle',0),'timestamp':hb.get('timestamp'),
                  'checks':sorted(completed),'check_count':check_count,
                  'reports':reports,'interrupted':interrupted}))
""" + "\nPY", timeout=90))


def wait_deacon_patrol(lab, *, after=None, timeout=600):
    before = after or {"cycle": 0, "reports": 0, "check_count": 0}
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        evidence = deacon_patrol(lab)
        assert not evidence["interrupted"], "Deacon self-sling interrupted its caller"
        if (evidence["cycle"] > max(1, before["cycle"])
                and evidence["reports"] > before["reports"]
                and evidence["check_count"] >= before["check_count"] + 3
                and len(evidence["checks"]) >= 3):
            return evidence
        time.sleep(5)
    raise AssertionError("Deacon did not complete a real patrol and advance its heartbeat")


def wait_deacon_startup(lab, timeout=180):
    # Startup ends once the new Deacon is doing patrol work. A complete cycle
    # and continued progress after handoff are checked separately above.
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        evidence = deacon_patrol(lab)
        assert not evidence["interrupted"], "Deacon self-sling interrupted its caller"
        if evidence["cycle"] > 1 and len(evidence["checks"]) >= 3:
            return evidence
        time.sleep(5)
    raise AssertionError("Fresh Deacon did not advance real patrol checks and its heartbeat")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--auth-lab", required=True)
    parser.add_argument("--taxiway", default="./taxiway")
    parser.add_argument("--repo", default="https://github.com/octocat/Hello-World")
    args = parser.parse_args()
    validate_context()
    with temporary_lab("gastown", auth_lab=args.auth_lab, repo=args.repo,
                       taxiway=args.taxiway, timeout=1200) as lab:
        command([args.taxiway, "verify", lab], timeout=180)
        socket, checked = sessions(lab)
        assert checked["model"] == "claude-opus-5-5", "Unexpected shipped Gastown default"
        snapshot = status(lab)
        assert snapshot.get("rigs"), "Repository rig missing"
        assert guest(lab, 'find /lab/work/gt -path "*/crew/*/README*" -type f -print -quit').strip(), "Crew repository missing"
        print(f"PASS Gastown startup and workspace: {checked['checked']} live roles, model={checked['model']}", flush=True)

        patrol = wait_deacon_patrol(lab)
        print("PASS Deacon startup: successful patrol checks/report and advancing heartbeat", flush=True)
        before_deacon = guest(lab, f"tmux -L {shlex.quote(socket)} display-message -p -t hq-deacon '#{{pane_pid}}'").strip()
        handoff_deacon = 'cd /lab/work/gt/deacon && export PATH="$HOME/.local/bin:$PATH" GT_ROLE=deacon TMUX_PANE=\'#{pane_id}\'; gt handoff deacon --watch=false --yes --no-git-check >/tmp/taxiway-deacon-handoff-check.log 2>&1'
        guest(lab, "tmux -L " + shlex.quote(socket) + " run-shell -b -t hq-deacon " + shlex.quote(handoff_deacon))
        deadline = time.monotonic() + 60
        while time.monotonic() < deadline:
            pane = guest(lab, f"tmux -L {shlex.quote(socket)} display-message -p -t hq-deacon '#{{pane_pid}}|#{{pane_dead}}|#{{pane_current_command}}'").decode().strip().split("|")
            if len(pane) == 3 and pane[0].encode() != before_deacon and pane[1] == "0" and pane[2] in ("node", "claude"):
                break
            time.sleep(1)
        else:
            raise AssertionError("Deacon handoff did not replace the live Claude process")
        wait_deacon_patrol(lab, after=patrol)
        print("PASS Deacon handoff: replacement completes patrol and heartbeat advances", flush=True)

        # Real remote handoff through Gastown's tmux restart builder.
        before = guest(lab, f"tmux -L {shlex.quote(socket)} display-message -p -t hq-mayor '#{{pane_pid}}'").strip()
        handoff = 'cd /lab/work/gt && export PATH="$HOME/.local/bin:$PATH" GT_ROLE=mayor TMUX_PANE=\'#{pane_id}\'; gt handoff mayor --watch=false --yes --no-git-check >/tmp/taxiway-handoff-check.log 2>&1'
        guest(lab, "tmux -L " + shlex.quote(socket) + " run-shell -b -t hq-mayor " + shlex.quote(handoff))
        deadline = time.monotonic() + 60
        while time.monotonic() < deadline:
            pane = guest(lab, f"tmux -L {shlex.quote(socket)} display-message -p -t hq-mayor '#{{pane_pid}}|#{{pane_dead}}|#{{pane_current_command}}'").decode().strip().split("|")
            if len(pane) == 3 and pane[0].encode() != before and pane[1] == "0" and pane[2] in ("node", "claude"):
                break
            time.sleep(1)
        else:
            raise AssertionError("Mayor handoff did not replace the live Claude process")
        _, after = sessions(lab)
        assert after["model"] == checked["model"], "Handoff changed the selected model"
        print("PASS Gastown Mayor handoff: new Claude process, model/aliases/gateway retained", flush=True)

        # Stop patrols before the bounded scenario to keep subscription usage small.
        guest(lab, 'export PATH="$HOME/.local/bin:$PATH"; cd /lab/work/gt; gt down >/tmp/taxiway-down-check.log 2>&1', timeout=120)
        agents={"sonnet-worker":{"description":"Invoke for the explicit Taxiway delegation test.","prompt":"Use Bash to run curl --max-time 20 -fsSI https://example.com > /lab/work/gt/network-proof. Then reply GASTOWN_CHILD_OK.","tools":["Bash"],"model":"sonnet"}}
        argv=["env","-u","CLAUDE_CODE_SUBAGENT_MODEL","-u","CLAUDE_CODE_SUBAGENT_MODEL_FORCE","CLAUDE_AGENT_SDK_DISABLE_BUILTIN_AGENTS=1",
              "/lab/orchestrators/gastown/launch-agent.sh","/lab/work/gt","claude","-p",
              "Invoke sonnet-worker once and wait for its result. Then use Bash to run: printf GASTOWN_TOOL_OK > /lab/work/gt/live-proof.txt; printf 'print(42)\\n' > /lab/work/gt/build-proof.py; python3 -m py_compile /lab/work/gt/build-proof.py. Finally reply exactly GASTOWN_OK.",
              "--model",checked["model"],"--output-format","stream-json","--verbose","--forward-subagent-text",
              "--no-session-persistence","--effort","low",
              "--tools","Agent,Bash","--max-budget-usd","2","--agents",json.dumps(agents)]
        # Deliberately omit injected gateway env: exercise the actual Gastown launcher.
        output=guest(lab,"timeout 180s "+shlex.join(argv),timeout=210,workdir="/lab/work/gt")
        events=[json.loads(line) for line in output.splitlines() if line.startswith(b"{")]
        result=[e for e in events if e.get("type")=="result"][-1]
        assert result.get("subtype")=="success" and not result.get("is_error"), "Gastown launcher client failed"
        assert "GASTOWN_OK" in result.get("result",""), "Final marker missing"
        assert not result.get("permission_denials"), "Tool permission denied"
        models=set(); nested=set(); tools=set(); spawned=set()
        for event in events:
            message=event.get("message",{})
            if event.get("type")=="assistant":
                model=message.get("model")
                if model:
                    models.add(model)
                    if event.get("parent_tool_use_id"): nested.add(model)
                for block in message.get("content",[]):
                    if block.get("type")=="tool_use":
                        tools.add(block.get("name"))
                        if block.get("name")=="Agent": spawned.add(block.get("input",{}).get("subagent_type"))
        assert checked["model"] in models, "Principal model was not used"
        assert "claude-sonnet-5-5" in nested and "sonnet-worker" in spawned, "Sonnet delegation not evidenced"
        assert "Bash" in tools, "No actual Bash tool invocation"
        assert guest(lab,"cat /lab/work/gt/live-proof.txt").strip()==b"GASTOWN_TOOL_OK", "Tool did not create proof file"
        assert guest(lab,"test -s /lab/work/gt/network-proof && find /lab/work/gt/__pycache__ -name 'build-proof*' -print -quit").strip(), "Delegated network/build effects missing"
        print("PASS Gastown launcher inference: Opus principal, Sonnet subagent, actual tool/file effect",flush=True)
    # A separate fresh installation catches startup failures hidden by an
    # already-populated hook or a recovered Deacon from the first lab.
    with temporary_lab("gastown", auth_lab=args.auth_lab, repo=args.repo,
                       taxiway=args.taxiway, timeout=1200) as lab:
        sessions(lab)
        wait_deacon_startup(lab)
        guest(lab, 'export PATH="$HOME/.local/bin:$PATH"; cd /lab/work/gt; gt down >/tmp/taxiway-down-check.log 2>&1', timeout=120)
        print("PASS Deacon second fresh startup: real patrol checks and advancing heartbeat", flush=True)
    print("All Gastown live cases passed; temporary labs removed; reference lab preserved.",flush=True)


if __name__ == "__main__":
    try:
        main()
    except (AssertionError,RuntimeError,OSError,subprocess.TimeoutExpired,IndexError):
        # Do not print captured output, argv or parsed request payloads.
        print("FAIL Gastown live scenario; no credential-bearing output displayed",file=sys.stderr)
        sys.exit(1)
