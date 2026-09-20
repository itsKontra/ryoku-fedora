#!/usr/bin/env python3
"""Submit validated SRPMs and collect only their successful COPR results."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import time
import urllib.parse
import urllib.request


def verify_sources(directory):
    metadata = json.loads((directory / 'release.json').read_text())
    expected = metadata['sources']
    actual = {p.name: hashlib.sha256(p.read_bytes()).hexdigest()
              for p in directory.glob('*.src.rpm')}
    if not actual or actual != expected:
        raise ValueError('SRPM set or checksums differ from the validated source manifest')
    return metadata


def wait_for_build(client, build_id, chroot, deadline, sleep=time.sleep):
    while time.monotonic() < deadline:
        build = client.build_proxy.get(build_id)
        state = build['state']
        print(f'COPR build {build_id}: {state}', flush=True)
        if state == 'succeeded':
            result = client.build_chroot_proxy.get(build_id, chroot)
            if result['state'] != 'succeeded':
                raise RuntimeError(f'COPR build {build_id} did not succeed in {chroot}')
            return
        if state not in ('importing', 'pending', 'starting', 'running', 'waiting', 'forked'):
            raise RuntimeError(f'COPR build {build_id} ended with state {state}')
        sleep(20)
    raise TimeoutError(f'COPR build {build_id} timed out; channel was not promoted')


def collect(client, build_id, chroot, out):
    urls = client.build_chroot_proxy.get_results_urls(build_id, chroot)
    if not urls:
        raise RuntimeError(f'COPR build {build_id} produced no RPMs')
    checksums = {}
    for url in urls:
        parsed = urllib.parse.urlparse(url)
        name = Path(parsed.path).name
        if parsed.scheme != 'https' or not name.endswith('.rpm') or name.endswith('.src.rpm'):
            raise ValueError(f'unexpected COPR result URL: {url}')
        target = out / name
        if target.exists():
            raise ValueError(f'duplicate COPR output: {name}')
        with urllib.request.urlopen(url, timeout=120) as response, target.open('wb') as stream:
            if urllib.parse.urlsplit(response.url).scheme != 'https':
                raise ValueError('COPR result redirected outside HTTPS')
            while chunk := response.read(1024 * 1024):
                stream.write(chunk)
        checksums[name] = hashlib.sha256(target.read_bytes()).hexdigest()
    return checksums


def main():
    from copr.v3 import Client
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('sources', type=Path)
    parser.add_argument('out', type=Path)
    parser.add_argument('--owner', default='itskontra')
    parser.add_argument('--project', default='ryoku')
    parser.add_argument('--chroot', default='fedora-44-x86_64')
    parser.add_argument('--timeout', type=int, default=18000)
    args = parser.parse_args()
    metadata = verify_sources(args.sources)
    args.out.mkdir(parents=True, exist_ok=False)
    client = Client.create_from_config_file(os.environ['COPR_CONFIG_FILE'])
    builds = []
    metadata['builds'] = builds
    metadata['rpms'] = {}
    report = args.out / 'release.json'
    deadline = time.monotonic() + args.timeout
    for name in sorted(metadata['sources']):
        build = client.build_proxy.create_from_file(
            args.owner, args.project, str(args.sources / name),
            buildopts={'chroots': [args.chroot], 'enable_net': False})
        build_id = build['id']
        url = f'https://copr.fedorainfracloud.org/coprs/{args.owner}/{args.project}/build/{build_id}/'
        builds.append(dict(id=build_id, source=name, chroot=args.chroot, url=url))
        report.write_text(json.dumps(metadata, indent=2) + '\n')
        print(url, flush=True)
        if summary := os.environ.get('GITHUB_STEP_SUMMARY'):
            with open(summary, 'a') as stream:
                stream.write(f'- [{name}]({url})\n')
    for build in builds:
        wait_for_build(client, build['id'], args.chroot, deadline)
        metadata['rpms'].update(collect(client, build['id'], args.chroot, args.out))
        report.write_text(json.dumps(metadata, indent=2) + '\n')


if __name__ == '__main__':
    main()
