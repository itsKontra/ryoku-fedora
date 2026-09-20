#!/usr/bin/env python3
"""Request COPR repository publication after the candidate passes install tests."""
import json
import os
from pathlib import Path
import sys


def publish(client, metadata):
    target = metadata['copr']
    owner, project, chroot = (target[k] for k in ('owner', 'project', 'chroot'))
    if (owner, project, chroot) != ('itskontra', 'ryoku', 'fedora-44-x86_64'):
        raise ValueError('unexpected publication target')
    project_info = client.project_proxy.get(owner, project)
    if not (project_info.get('devel_mode') or project_info.get('disable_createrepo')):
        raise ValueError('COPR must use manual repository generation')
    builds = metadata['builds']
    ids = {b['id'] for b in builds}
    if not ids or len(ids) != len(builds) or len(builds) != len(metadata['sources']):
        raise ValueError('incomplete build manifest')
    if {b['source'] for b in builds} != set(metadata['sources']):
        raise ValueError('builds do not cover the source manifest')
    for build_id in ids:
        build = client.build_proxy.get(build_id)
        if (build['ownername'], build['projectname'], build['state']) != (owner, project, 'succeeded'):
            raise ValueError('candidate build is not a successful build in the target project')
        if client.build_chroot_proxy.get(build_id, chroot)['state'] != 'succeeded':
            raise ValueError('candidate chroot failed')
    # A stale retry must never expose a newer, untested submission. This project
    # is dedicated to the serialized workflow; manual builds need their own project.
    recent = client.build_proxy.get_list(owner, project,
        pagination={'limit': len(ids) + 1, 'order': 'id', 'order_type': 'DESC'})
    if {b['id'] for b in recent if b['id'] >= min(ids)} != ids:
        raise ValueError('interleaved or newer builds exist; run the complete pipeline again')
    client.project_proxy.regenerate_repos(owner, project)
    print('COPR repository generation requested for the tested candidate')


if __name__ == '__main__':
    from copr.v3 import Client
    client = Client.create_from_config_file(os.environ['COPR_CONFIG_FILE'])
    publish(client, json.loads((Path(sys.argv[1]) / 'release.json').read_text()))
