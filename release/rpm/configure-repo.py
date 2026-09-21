#!/usr/bin/env python3
"""Install the Ryoku COPR repository on a mutable Fedora host."""
import argparse
import importlib.machinery
import os
from pathlib import Path
import tempfile
import urllib.parse
import urllib.request

verify = importlib.machinery.SourceFileLoader(
    'verify_rpms', str(Path(__file__).with_name('verify-rpms.py'))).load_module()


COPR_ROOT = 'https://download.copr.fedorainfracloud.org/results/itskontra/ryoku'


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


def configure(copr_fingerprint, root=Path('/')):
    with tempfile.TemporaryDirectory() as work:
        with urllib.request.urlopen(COPR_ROOT + '/pubkey.gpg', timeout=30) as response:
            if urllib.parse.urlsplit(response.url).scheme != 'https':
                raise ValueError('key download redirected outside HTTPS')
            data = response.read(1024 * 1024 + 1)
            if len(data) > 1024 * 1024:
                raise ValueError('public key exceeds size limit')
        key = Path(work) / 'copr.asc'
        key.write_bytes(data)
        verify.verify_key(key, copr_fingerprint)
    key_path = 'etc/pki/rpm-gpg/RPM-GPG-KEY-ryoku-copr'
    atomic_write(root / key_path, data)
    template = Path(__file__).with_name('ryoku.repo.in').read_text()
    config = template.replace('@BASE_URL@', COPR_ROOT).replace('@KEY_URL@', 'file:///' + key_path)
    atomic_write(root / 'etc/yum.repos.d/RyokuCOPR.repo', config.encode())
    (root / 'etc/yum.repos.d/ryoku.repo').unlink(missing_ok=True)
    (root / 'etc/dnf/vars/ryoku_baseurl').unlink(missing_ok=True)


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('copr_fingerprint')
    args = parser.parse_args()
    configure(args.copr_fingerprint)
