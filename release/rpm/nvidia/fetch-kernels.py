#!/usr/bin/env python3
"""Download signed kernel-devel for the newest Fedora kernels a ryoku-nvidia build targets.

Three kernels, the default installonly_limit: every kernel a Fedora machine can
still boot gets a module built against the same driver as its userspace.
"""
import argparse
import functools
import subprocess
import urllib.parse
import urllib.request
import xmlrpc.client
from pathlib import Path

import rpm

KOJI_HUB = 'https://koji.fedoraproject.org/kojihub'
KOJI_PACKAGES = 'https://kojipkgs.fedoraproject.org/packages'


def newest(builds, count):
    evr = lambda b: ('0', b['version'], b['release'])
    order = functools.cmp_to_key(lambda a, b: rpm.labelCompare(evr(a), evr(b)))
    return sorted(builds, key=order, reverse=True)[:count]


def signed_url(build, arch, key_id):
    version, release = build['version'], build['release']
    name = f'kernel-devel-{version}-{release}.{arch}.rpm'
    return f'{KOJI_PACKAGES}/kernel/{version}/{release}/data/signed/{key_id}/{arch}/{name}'


def download(url, target):
    with urllib.request.urlopen(url, timeout=300) as response, target.open('wb') as stream:
        if urllib.parse.urlsplit(response.url).scheme != 'https':
            raise ValueError('kernel download redirected outside HTTPS')
        while chunk := response.read(1024 * 1024):
            stream.write(chunk)


def verified(path):
    result = subprocess.run(['rpmkeys', '--checksig', str(path)], capture_output=True, text=True)
    return result.returncode == 0 and 'signatures OK' in result.stdout


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('out', type=Path)
    parser.add_argument('--tag', default='f44-updates')
    parser.add_argument('--arch', default='x86_64')
    parser.add_argument('--key-id', required=True, help='short ID of the Fedora release key the RPMs must carry')
    parser.add_argument('--count', type=int, default=3)
    args = parser.parse_args()
    args.out.mkdir(parents=True, exist_ok=False)
    hub = xmlrpc.client.ServerProxy(KOJI_HUB, allow_none=True)
    builds = hub.listTagged(args.tag, None, True, None, False, 'kernel')
    for build in newest(builds, args.count):
        target = args.out / f"kernel-devel-{build['version']}-{build['release']}.{args.arch}.rpm"
        download(signed_url(build, args.arch, args.key_id.lower()), target)
        if not verified(target):
            raise RuntimeError(f'{target.name} is not signed by the Fedora release key')
        print(f"{build['version']}-{build['release']}.{args.arch}")


if __name__ == '__main__':
    main()
