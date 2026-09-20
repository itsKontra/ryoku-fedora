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
    raise TimeoutError(f'COPR build {build_id} timed out; repository was not published')


def collect(client, build_id, chroot, out, project_info=None):
    build = client.build_proxy.get(build_id)
    built_data = client.build_proxy.get_built_packages(build_id)
    packages = built_data.get(chroot, {}).get('packages', [])
    binary_packages = [p for p in packages if p.get('arch') != 'src']
    if not binary_packages:
        raise RuntimeError(f'COPR build {build_id} produced no binary RPMs in {chroot}')

    owner = build['ownername']
    project = build['projectname']
    repo_url = build.get('repo_url') or f'https://download.copr.fedorainfracloud.org/results/{owner}/{project}'

    if project_info is None:
        project_info = client.project_proxy.get(owner, project)

    is_devel = bool(project_info.get('devel_mode'))
    repo_chroot = f'{chroot}-devel' if is_devel else chroot
    repo_base = f"{repo_url.rstrip('/')}/{repo_chroot}"

    chroot_info = client.build_chroot_proxy.get(build_id, chroot)
    result_url = chroot_info.get('result_url', '').rstrip('/')

    checksums = {}
    for p in binary_packages:
        name = p['name']
        filename = f"{name}-{p['version']}-{p['release']}.{p['arch']}.rpm"
        initial = name[0].lower()

        candidates = [
            f"{repo_base}/Packages/{initial}/{filename}",
            f"{result_url}/{filename}",
            f"{repo_base}/{filename}",
        ]
        target = out / filename
        if target.exists():
            raise ValueError(f'duplicate COPR output: {filename}')

        downloaded = False
        last_error = None
        for url in candidates:
            if not url or not url.startswith('https://'):
                continue
            try:
                with urllib.request.urlopen(url, timeout=120) as response, target.open('wb') as stream:
                    if urllib.parse.urlsplit(response.url).scheme != 'https':
                        raise ValueError('COPR result redirected outside HTTPS')
                    while chunk := response.read(1024 * 1024):
                        stream.write(chunk)
                downloaded = True
                break
            except Exception as exc:
                last_error = exc
                if target.exists():
                    target.unlink()

        if not downloaded:
            raise RuntimeError(f'failed to download {filename} from COPR: {last_error}')

        checksums[filename] = hashlib.sha256(target.read_bytes()).hexdigest()

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
    project_info = client.project_proxy.get(args.owner, args.project)
    if not (project_info.get('devel_mode') or project_info.get('disable_createrepo')):
        raise ValueError('enable manual repository generation before submitting builds')
    metadata['copr'] = dict(owner=args.owner, project=args.project, chroot=args.chroot)
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
        metadata['rpms'].update(collect(client, build['id'], args.chroot, args.out, project_info=project_info))
        report.write_text(json.dumps(metadata, indent=2) + '\n')


if __name__ == '__main__':
    main()
