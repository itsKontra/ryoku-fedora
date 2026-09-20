#!/usr/bin/env python3
"""Copy a gated candidate to the HTTPS release host, then promote it."""
import json
import os
from pathlib import Path
import re
import shlex
import subprocess
import sys
import tempfile


def publish(candidate):
    base = os.environ['RYOKU_RPM_BASE_URL']
    host = os.environ['RYOKU_RPM_PUBLISH_HOST']
    root = os.environ['RYOKU_RPM_PUBLISH_ROOT']
    if not base.startswith('https://') or any(c.isspace() for c in base):
        raise ValueError('RYOKU_RPM_BASE_URL must be an HTTPS URL without whitespace')
    if not re.fullmatch(r'[A-Za-z0-9_][A-Za-z0-9_.@-]*', host):
        raise ValueError('invalid SSH publish host')
    if not re.fullmatch(r'/[A-Za-z0-9_./-]+', root) or '..' in root.split('/'):
        raise ValueError('publish root must be an absolute path without traversal')
    metadata = json.loads((candidate / 'release.json').read_text())
    release = metadata['release']
    if not re.fullmatch(r'v[0-9][A-Za-z0-9._-]*', release):
        raise ValueError('invalid release name')
    incoming = root.rstrip('/') + '/.incoming/' + release
    with tempfile.TemporaryDirectory() as directory:
        key = Path(directory) / 'key'
        key.write_text(os.environ['RYOKU_RPM_SSH_KEY'] + '\n')
        key.chmod(0o600)
        known = Path(directory) / 'known_hosts'
        known.write_text(os.environ['RYOKU_RPM_KNOWN_HOSTS'] + '\n')
        ssh = ['ssh', '-i', str(key), '-o', 'BatchMode=yes', '-o', 'IdentitiesOnly=yes',
               '-o', 'StrictHostKeyChecking=yes', '-o', f'UserKnownHostsFile={known}']
        command = f'mkdir -p {shlex.quote(root + "/.incoming")} && mkdir {shlex.quote(incoming)}'
        subprocess.run(ssh + [host, command], check=True)
        subprocess.run(['rsync', '-r', '--checksum', '-e', shlex.join(ssh),
                        str(candidate) + '/', host + ':' + incoming + '/'], check=True)
        script = Path(__file__).with_name('promote.py').read_bytes()
        command = shlex.join(['python3', '-', root, incoming, metadata['channel'], release, base])
        subprocess.run(ssh + [host, command], input=script, check=True)


if __name__ == '__main__':
    publish(Path(sys.argv[1]))
