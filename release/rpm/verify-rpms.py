#!/usr/bin/env python3
"""Verify the complete COPR RPM set using an isolated RPM trust database."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import subprocess
import tempfile


def run(*args):
    return subprocess.check_output(args, text=True, env=dict(os.environ, LC_ALL='C')).strip()


def verify_key(key, fingerprint):
    if not fingerprint or len(fingerprint.replace(' ', '')) < 40:
        raise ValueError('a full public-key fingerprint is required')
    with tempfile.TemporaryDirectory() as home:
        listing = run('gpg', '--homedir', home, '--batch', '--with-colons', '--show-keys', str(key))
    fingerprints = []
    primary = False
    for line in listing.splitlines():
        fields = line.split(':')
        if fields[0] == 'pub':
            primary = True
        elif fields[0] == 'fpr' and primary:
            fingerprints.append(fields[9].upper())
            primary = False
    if fingerprints != [fingerprint.replace(' ', '').upper()]:
        raise ValueError(f'unexpected signing key in {key}: {fingerprints}')


def verify(directory, key, fingerprint):
    verify_key(key, fingerprint)
    metadata = json.loads((directory / 'release.json').read_text())
    actual = {p.name: hashlib.sha256(p.read_bytes()).hexdigest() for p in directory.glob('*.rpm')}
    if not actual or actual != metadata['rpms']:
        raise ValueError('RPM set or checksums differ from the COPR build manifest')
    with tempfile.TemporaryDirectory() as database:
        run('rpm', '--dbpath', database, '--import', str(key))
        for name in actual:
            package = directory / name
            result = run('rpmkeys', '--dbpath', database, '--checksig', str(package))
            if 'signatures OK' not in result:
                raise ValueError(f'missing or invalid RPM signature: {result}')
            version = run('rpm', '--dbpath', database, '-qp', '--qf', '%{VERSION}', str(package))
            if version != metadata['version']:
                raise ValueError(f'{name} has version {version}, expected {metadata["version"]}')


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('directory', type=Path)
    parser.add_argument('key', type=Path)
    parser.add_argument('fingerprint')
    args = parser.parse_args()
    verify(args.directory, args.key, args.fingerprint)
