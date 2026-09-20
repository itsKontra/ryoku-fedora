#!/usr/bin/env python3
"""Promote a tested repository on the release host using atomic symlinks."""
import argparse
import fcntl
import json
import os
from pathlib import Path
import re


def atomic_json(path, data):
    temporary = path.with_suffix('.tmp')
    temporary.write_text(json.dumps(data, indent=2) + '\n')
    os.replace(temporary, path)


def promote(root, candidate, channel, release, base_url):
    if channel not in ('testing', 'stable') or not re.fullmatch(r'v[0-9]+\.[0-9]+\.[0-9]+(-(alpha|beta|rc)\.[0-9]+)?', release):
        raise ValueError('invalid channel or release name')
    if not base_url.startswith('https://'):
        raise ValueError('an HTTPS release base URL is required')
    metadata = json.loads((candidate / 'release.json').read_text())
    if metadata['release'] != release or metadata['channel'] != channel:
        raise ValueError('candidate metadata does not match requested promotion')
    for path in ('repodata/repomd.xml', 'repodata/repomd.xml.asc', 'keys/copr.asc', 'keys/metadata.asc'):
        if not (candidate / path).is_file():
            raise ValueError(f'incomplete candidate: {path}')
    root.mkdir(parents=True, exist_ok=True)
    with (root / '.publish.lock').open('a') as lock:
        fcntl.flock(lock, fcntl.LOCK_EX)
        pointer = root / 'channels' / channel
        if pointer.exists():
            previous = json.loads((pointer / '44/x86_64/release.json').read_text())
            if int(previous['sequence']) >= int(metadata['sequence']):
                raise ValueError('refusing to replace a channel with an older or repeated run')
        destination = root / 'releases' / release
        if destination.exists():
            raise ValueError('frozen release already exists')
        destination.parent.mkdir(parents=True, exist_ok=True)
        staging = root / '.incoming' / (release + '.tree')
        (staging / '44').mkdir(parents=True, exist_ok=False)
        os.rename(candidate, staging / '44/x86_64')
        os.rename(staging, destination)
        index = root / 'releases/index.json'
        ledger = json.loads(index.read_text()) if index.exists() else {'releases': []}
        if channel == 'stable':
            ledger['latest'] = release
            ledger['releases'].insert(0, dict(tag=release, name=metadata.get('name', ''),
                version=metadata['version'], commit=metadata['commit'], date=metadata['date'],
                repo=base_url.rstrip('/') + '/releases/' + release))
            atomic_json(index, ledger)
        pointer.parent.mkdir(parents=True, exist_ok=True)
        temporary = pointer.with_suffix('.next')
        temporary.unlink(missing_ok=True)
        temporary.symlink_to(Path('../releases') / release)
        os.replace(temporary, pointer)


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('root', type=Path)
    parser.add_argument('candidate', type=Path)
    parser.add_argument('channel')
    parser.add_argument('release')
    parser.add_argument('base_url')
    args = parser.parse_args()
    promote(args.root, args.candidate, args.channel, args.release, args.base_url)
