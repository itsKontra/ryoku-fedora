#!/usr/bin/env python3
"""Install the signed Ryoku channel repository on a mutable Fedora host."""
import argparse
import importlib.machinery
import os
from pathlib import Path
import tempfile
import urllib.parse
import urllib.request

verify = importlib.machinery.SourceFileLoader(
    'verify_rpms', str(Path(__file__).with_name('verify-rpms.py'))).load_module()


def validate_base(base):
    url = urllib.parse.urlsplit(base)
    if url.scheme != 'https' or not url.hostname or url.username or url.password or url.query or url.fragment:
        raise ValueError('RYOKU_RPM_BASE_URL must be an HTTPS repository root')
    if any(c.isspace() for c in base):
        raise ValueError('repository root must not contain whitespace')
    return base.rstrip('/')


def atomic_write(path, data):
    path.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.NamedTemporaryFile(dir=path.parent, delete=False) as stream:
        temporary = Path(stream.name)
        stream.write(data)
    try:
        temporary.chmod(0o644)
        os.replace(temporary, path)
    finally:
        temporary.unlink(missing_ok=True)


def configure(base, copr_fingerprint, metadata_fingerprint, channel, root=Path('/')):
    base = validate_base(base)
    if channel not in ('stable', 'testing'):
        raise ValueError('channel must be stable or testing')
    key_root = f'{base}/channels/{channel}/44/x86_64/keys'
    keys = {}
    with tempfile.TemporaryDirectory() as work:
        for name, fingerprint in [('copr', copr_fingerprint), ('metadata', metadata_fingerprint)]:
            with urllib.request.urlopen(f'{key_root}/{name}.asc', timeout=30) as response:
                if urllib.parse.urlsplit(response.url).scheme != 'https':
                    raise ValueError('key download redirected outside HTTPS')
                data = response.read(1024 * 1024 + 1)
                if len(data) > 1024 * 1024:
                    raise ValueError('public key exceeds size limit')
            key = Path(work) / name
            key.write_bytes(data)
            verify.verify_key(key, fingerprint)
            keys[name] = data
    for name, data in keys.items():
        atomic_write(root / f'etc/pki/rpm-gpg/RPM-GPG-KEY-ryoku-{name}', data)
    template = Path(__file__).with_name('ryoku.repo.in').read_text()
    key_urls = ' '.join(f'file:///etc/pki/rpm-gpg/RPM-GPG-KEY-ryoku-{name}' for name in keys)
    config = template.replace('@BASE_URL@', base).replace('@KEY_URL@', key_urls)
    config = config.replace('/channels/testing/', f'/channels/{channel}/')
    atomic_write(root / 'etc/dnf/vars/ryoku_baseurl', (base + '\n').encode())
    atomic_write(root / 'etc/yum.repos.d/ryoku.repo', config.encode())


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('base_url')
    parser.add_argument('copr_fingerprint')
    parser.add_argument('metadata_fingerprint')
    parser.add_argument('--channel', choices=['stable', 'testing'], default='testing')
    args = parser.parse_args()
    configure(args.base_url, args.copr_fingerprint, args.metadata_fingerprint, args.channel)
