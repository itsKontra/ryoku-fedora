import importlib.util
import json
import os
from pathlib import Path
import tempfile
import subprocess
import time
import unittest
from unittest.mock import Mock, patch

ROOT = Path(__file__).resolve().parents[1]


def module(name):
    spec = importlib.util.spec_from_file_location(name, ROOT / (name + '.py'))
    result = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(result)
    return result


copr = module('copr-build')
publisher = module('publish')
repository = module('configure-repo')


class CoprBuilds(unittest.TestCase):
    def test_canceled_and_failed_chroot_do_not_pass(self):
        client = Mock()
        client.build_proxy.get.return_value = {'state': 'canceled'}
        with self.assertRaises(RuntimeError):
            copr.wait_for_build(client, 1, 'fedora-44-x86_64', time.monotonic() + 10)
        client.build_proxy.get.return_value = {'state': 'succeeded'}
        client.build_chroot_proxy.get.return_value = {'state': 'failed'}
        with self.assertRaises(RuntimeError):
            copr.wait_for_build(client, 1, 'fedora-44-x86_64', time.monotonic() + 10)

    def test_waits_for_success(self):
        client = Mock()
        client.build_proxy.get.side_effect = [{'state': 'running'}, {'state': 'succeeded'}]
        client.build_chroot_proxy.get.return_value = {'state': 'succeeded'}
        sleeper = Mock()
        copr.wait_for_build(client, 1, 'fedora-44-x86_64', time.monotonic() + 10, sleeper)
        sleeper.assert_called_once_with(20)

    def test_timeout_does_not_pass(self):
        with self.assertRaises(TimeoutError):
            copr.wait_for_build(Mock(), 1, 'fedora-44-x86_64', time.monotonic() - 1)

    def test_tampered_or_added_sources_fail(self):
        import hashlib
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            source = root / 'one.src.rpm'
            source.write_bytes(b'validated source')
            (root / 'release.json').write_text(json.dumps({'sources': {
                source.name: hashlib.sha256(source.read_bytes()).hexdigest()}}))
            copr.verify_sources(root)
            source.write_bytes(b'changed after gate')
            with self.assertRaises(ValueError):
                copr.verify_sources(root)

    def test_collect_downloads_binary_rpms_and_computes_checksums(self):
        import hashlib
        client = Mock()
        client.build_proxy.get.return_value = {
            'ownername': 'itskontra',
            'projectname': 'ryoku',
            'repo_url': 'https://download.copr.fedorainfracloud.org/results/itskontra/ryoku',
        }
        client.build_proxy.get_built_packages.return_value = {
            'fedora-44-x86_64': {
                'packages': [
                    {'name': 'pkg', 'version': '1.0', 'release': '1.fc44', 'arch': 'src'},
                    {'name': 'pkg', 'version': '1.0', 'release': '1.fc44', 'arch': 'x86_64'},
                ]
            }
        }
        client.build_chroot_proxy.get.return_value = {
            'result_url': 'https://download.copr.fedorainfracloud.org/results/itskontra/ryoku/fedora-44-x86_64/1-pkg'
        }
        project_info = {'devel_mode': True}

        rpm_bytes = b'fake-binary-rpm-content'
        expected_hash = hashlib.sha256(rpm_bytes).hexdigest()

        response = Mock()
        response.url = 'https://download.copr.fedorainfracloud.org/results/itskontra/ryoku/fedora-44-x86_64-devel/Packages/p/pkg-1.0-1.fc44.x86_64.rpm'
        response.read.side_effect = [rpm_bytes, b'']

        with tempfile.TemporaryDirectory() as out_dir:
            out_path = Path(out_dir)
            with patch.object(copr.urllib.request, 'urlopen') as urlopen_mock:
                urlopen_mock.return_value.__enter__.return_value = response
                csums = copr.collect(client, 1, 'fedora-44-x86_64', out_path, project_info=project_info)

            self.assertEqual(csums, {'pkg-1.0-1.fc44.x86_64.rpm': expected_hash})
            self.assertEqual((out_path / 'pkg-1.0-1.fc44.x86_64.rpm').read_bytes(), rpm_bytes)

    def test_collect_no_binary_rpms_raises(self):
        client = Mock()
        client.build_proxy.get.return_value = {'ownername': 'itskontra', 'projectname': 'ryoku'}
        client.build_proxy.get_built_packages.return_value = {
            'fedora-44-x86_64': {
                'packages': [
                    {'name': 'pkg', 'version': '1.0', 'release': '1.fc44', 'arch': 'src'},
                ]
            }
        }
        with tempfile.TemporaryDirectory() as out_dir:
            with self.assertRaises(RuntimeError):
                copr.collect(client, 1, 'fedora-44-x86_64', Path(out_dir))


class CleanBuildGate(unittest.TestCase):
    def test_build_failure_retains_logs_and_fails_the_gate(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            sources = root / 'sources'
            sources.mkdir()
            (sources / 'probe.src.rpm').write_bytes(b'test source')
            mock = root / 'mock'
            mock.write_text('''#!/usr/bin/env python3
import pathlib, sys
result = pathlib.Path(sys.argv[sys.argv.index('--resultdir') + 1])
result.mkdir(parents=True)
(result / 'build.log').write_text('missing dependency')
sys.exit(3)
''')
            mock.chmod(0o755)
            result = subprocess.run([str(ROOT / 'rebuild-srpms.sh'), str(sources), str(root / 'out')],
                env=dict(os.environ, PATH=str(root) + ':' + os.environ['PATH']), capture_output=True)
            self.assertEqual(result.returncode, 3, result.stderr)
            self.assertEqual((root / 'out/logs/probe/build.log').read_text(), 'missing dependency')

    def test_success_collects_every_binary_without_source_duplicates(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            sources = root / 'sources'
            sources.mkdir()
            for name in ('one', 'two'):
                (sources / (name + '.src.rpm')).write_bytes(b'test source')
            mock = root / 'mock'
            mock.write_text('''#!/usr/bin/env python3
import pathlib, sys
result = pathlib.Path(sys.argv[sys.argv.index('--resultdir') + 1])
result.mkdir(parents=True)
name = result.name
(result / 'build.log').write_text('built')
(result / (name + '.x86_64.rpm')).write_bytes(b'binary')
(result / (name + '.src.rpm')).write_bytes(b'source')
''')
            mock.chmod(0o755)
            result = subprocess.run([str(ROOT / 'rebuild-srpms.sh'), str(sources), str(root / 'out')],
                env=dict(os.environ, PATH=str(root) + ':' + os.environ['PATH']), capture_output=True)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(sorted(p.name for p in (root / 'out').rglob('*.rpm')),
                             ['one.x86_64.rpm', 'two.x86_64.rpm'])


class FedoraPayload(unittest.TestCase):
    def test_desktop_does_not_stage_removed_arch_boot_files(self):
        payload = (ROOT / 'payload/ryoku-desktop.sh').read_text()
        self.assertNotIn('system/boot/', payload)


class Publication(unittest.TestCase):
    def candidate(self):
        return dict(copr=dict(owner='itskontra', project='ryoku', chroot='fedora-44-x86_64'),
                    builds=[dict(id=10, source='one.src.rpm')], sources={'one.src.rpm': 'hash'})

    def client(self):
        client = Mock()
        client.project_proxy.get.return_value = {'disable_createrepo': True}
        client.build_proxy.get.return_value = dict(ownername='itskontra', projectname='ryoku', state='succeeded')
        client.build_chroot_proxy.get.return_value = {'state': 'succeeded'}
        client.build_proxy.get_list.return_value = [{'id': 10}, {'id': 9}]
        return client

    def test_tested_candidate_requests_publication(self):
        client = self.client()
        publisher.publish(client, self.candidate())
        client.project_proxy.regenerate_repos.assert_called_once_with('itskontra', 'ryoku')

    def test_newer_or_interleaved_build_prevents_stale_publication(self):
        client = self.client()
        client.build_proxy.get_list.return_value = [{'id': 11}, {'id': 10}]
        with self.assertRaises(ValueError): publisher.publish(client, self.candidate())
        client.project_proxy.regenerate_repos.assert_not_called()

    def test_automatic_publication_or_failed_build_rejected(self):
        for automatic in (True, False):
            client = self.client()
            if automatic: client.project_proxy.get.return_value = {'disable_createrepo': False}
            else: client.build_chroot_proxy.get.return_value = {'state': 'failed'}
            with self.assertRaises(ValueError): publisher.publish(client, self.candidate())
            client.project_proxy.regenerate_repos.assert_not_called()

    def test_incomplete_manifest_rejected(self):
        client = self.client()
        metadata = self.candidate()
        metadata['sources']['missing.src.rpm'] = 'hash'
        with self.assertRaises(ValueError): publisher.publish(client, metadata)
        client.project_proxy.regenerate_repos.assert_not_called()


class Repository(unittest.TestCase):
    def test_direct_copr_configuration_preserves_package_checks(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            legacy = root / 'etc/yum.repos.d/ryoku.repo'
            legacy.parent.mkdir(parents=True)
            legacy.write_text('[ryoku]\nbaseurl=https://example.invalid/old\n')
            response = Mock()
            response.url = repository.COPR_ROOT + '/pubkey.gpg'
            response.read.return_value = b'public key'
            with patch.object(repository.urllib.request, 'urlopen') as download:
                download.return_value.__enter__.return_value = response
                with patch.object(repository.verify, 'verify_key') as verify:
                    repository.configure('a' * 40, root)
                    verify.assert_called_once()
            config = (root / 'etc/yum.repos.d/RyokuCOPR.repo').read_text()
            self.assertIn('[RyokuCOPR]', config)
            self.assertFalse(legacy.exists())
            self.assertIn(repository.COPR_ROOT + '/fedora-$releasever-$basearch/', config)
            self.assertIn('gpgcheck=1', config)
            self.assertIn('repo_gpgcheck=0', config)
            self.assertNotIn('channels/', config)
            self.assertFalse((root / 'etc/dnf/vars/ryoku_baseurl').exists())

    def test_invalid_key_does_not_change_repository(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            response = Mock()
            response.url = 'https://example.invalid/key'
            response.read.return_value = b'wrong key'
            with patch.object(repository.urllib.request, 'urlopen') as download:
                download.return_value.__enter__.return_value = response
                with patch.object(repository.verify, 'verify_key', side_effect=ValueError('wrong fingerprint')):
                    with self.assertRaises(ValueError):
                        repository.configure('a' * 40, root)
            self.assertFalse((root / 'etc/yum.repos.d/RyokuCOPR.repo').exists())


if __name__ == '__main__':
    unittest.main()
