#!/usr/bin/env bash
# Run the real bootstrap against controlled package-manager and tool binaries.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/../../../.."
python3 - <<'PY'
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile

dispatch = r'''
import json, os, pathlib, sys
root = pathlib.Path(os.environ['BOOTSTRAP_FIXTURE'])
state = json.loads((root / 'state.json').read_text())
name = pathlib.Path(sys.argv[0]).name
args = sys.argv[1:]
with (root / 'calls').open('a') as log:
    log.write(name + ' ' + ' '.join(args) + '\n')
if name == 'sudo':
    if args[0] == '-E': args = args[1:]
    if args[0] == 'bash':
        sys.stdin.buffer.read()
        (root / 'setup').touch()
    elif args[:2] == ['test', '-s']:
        sys.exit(0 if state.get('key', True) else 1)
    elif args[0] == 'grep':
        sys.exit(0 if state.get('repo', True) else 1)
    elif args[0] == 'apt-get' and args[-1] == 'nodejs':
        (root / 'install-node').touch()
        for tool in ('node', 'npm'):
            path = root / 'bin' / tool
            if not path.exists(): path.symlink_to('fixture-tool')
        state['node'] = state.get('installed_node', 'v22.21.0')
        state['npm'] = True
        (root / 'state.json').write_text(json.dumps(state))
elif name == 'curl':
    if state.get('setup_download_failure'): sys.exit(22)
    print('# controlled setup payload')
elif name == 'apt-cache':
    print('nodejs:\n  Candidate: ' + state.get('candidate', '22.21.0-1nodesource1'))
elif name == 'node':
    print(state['node'])
elif name == 'npm':
    if state.get('npm_broken'): sys.exit(1)
    print('10.9.4')
else:
    if name == state.get('broken_tool'): sys.exit(1)
    print(name + ' fixture version')
'''

cases = [
    ('fresh', {}, True, True),
    ('healthy', {'node': 'v22.21.0', 'npm': True}, True, False),
    ('missing-npm', {'node': 'v22.21.0'}, True, True),
    ('wrong-version', {'node': 'v18.19.1', 'npm': True}, True, True),
    ('missing-key', {'key': False}, False, False),
    ('missing-repository', {'repo': False}, False, False),
    ('distribution-candidate', {'candidate': '18.19.1+dfsg-6ubuntu5'}, False, False),
    ('setup-download-fails', {'setup_download_failure': True}, False, False),
    ('wrong-installed-version', {'installed_node': 'v18.19.1'}, False, True),
    ('broken-npm', {'node': 'v22.21.0', 'npm': True, 'npm_broken': True}, False, True),
    ('missing-required-tool', {'node': 'v22.21.0', 'npm': True, 'missing': 'git'}, False, False),
    ('broken-required-tool', {'node': 'v22.21.0', 'npm': True, 'broken_tool': 'java'}, False, False),
]
for name, state, success, installs in cases:
    with tempfile.TemporaryDirectory(prefix='taxiway-bootstrap-') as directory:
        root = Path(directory)
        bin_dir = root / 'bin'
        bin_dir.mkdir()
        home = root / 'home'
        home.mkdir()
        (root / 'state.json').write_text(json.dumps(state))
        tool = bin_dir / 'fixture-tool'
        tool.write_text('#!' + sys.executable + '\n' + dispatch)
        tool.chmod(0o700)
        for command in ('bash', 'dirname', 'awk', 'grep', 'head', 'cat'):
            (bin_dir / command).symlink_to(shutil.which(command))
        for command in ('sudo', 'curl', 'apt-cache', 'docker', 'python3', 'java', 'git', 'tmux', 'asciinema'):
            if command != state.get('missing'):
                (bin_dir / command).symlink_to('fixture-tool')
        for command in ('node', 'npm'):
            if state.get(command): (bin_dir / command).symlink_to('fixture-tool')
        result = subprocess.run(['bash', 'infra/commands/bootstrap.sh'],
            env=dict(os.environ, PATH=str(bin_dir), HOME=str(home), BOOTSTRAP_FIXTURE=str(root)),
            stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
        assert (result.returncode == 0) == success, (name, result.returncode, result.stdout)
        assert (root / 'install-node').exists() == installs, (name, result.stdout)
        if success:
            assert 'Node.js 22 and a working npm are required' not in result.stdout
            assert (home / '.profile').exists(), (name, result.stdout)
        else:
            assert not (home / '.profile').exists(), (name, result.stdout)
        if name == 'healthy': assert not (root / 'setup').exists()
        print('PASS: bootstrap ' + name)
PY
