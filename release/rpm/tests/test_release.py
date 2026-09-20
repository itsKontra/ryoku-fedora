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
promotion = module('promote')
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


class Promotion(unittest.TestCase):
    def candidate(self, root, release, sequence, channel='testing'):
        candidate = root / '.incoming' / release
        candidate.mkdir(parents=True)
        (candidate / 'release.json').write_text(json.dumps(dict(
            release=release, sequence=sequence, channel=channel, version='0.123',
            commit='abc', date='2026-09-20T00:00:00Z')))
        for path in ('repodata/repomd.xml', 'repodata/repomd.xml.asc', 'keys/copr.asc', 'keys/metadata.asc'):
            target = candidate / path
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_text('test payload')
        return candidate

    def test_channel_moves_to_complete_set_and_old_release_survives(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            for number in (1, 2):
                name = f'v0.0.0-alpha.{number}'
                candidate = self.candidate(root, name, number)
                promotion.promote(root, candidate, 'testing', name, 'https://example.invalid/fedora')
            self.assertTrue((root / 'releases/v0.0.0-alpha.1/44/x86_64/release.json').is_file())
            current = root / 'channels/testing/44/x86_64/release.json'
            self.assertEqual(json.loads(current.read_text())['sequence'], 2)
            candidate = self.candidate(root, 'v0.0.0-alpha.0', 0)
            with self.assertRaises(ValueError):
                promotion.promote(root, candidate, 'testing', 'v0.0.0-alpha.0', 'https://example.invalid/fedora')
            self.assertEqual(json.loads(current.read_text())['sequence'], 2)

    def test_failed_candidate_never_replaces_channel(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            candidate = self.candidate(root, 'v1.0.0', 1, 'stable')
            (candidate / 'repodata/repomd.xml.asc').unlink()
            with self.assertRaises(ValueError):
                promotion.promote(root, candidate, 'stable', 'v1.0.0', 'https://example.invalid/fedora')
            self.assertFalse((root / 'channels/stable').exists())

    def test_stable_release_updates_rollback_ledger(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            candidate = self.candidate(root, 'v1.0.0', 1, 'stable')
            promotion.promote(root, candidate, 'stable', 'v1.0.0', 'https://example.invalid/fedora')
            ledger = json.loads((root / 'releases/index.json').read_text())
            self.assertEqual(ledger['latest'], 'v1.0.0')
            self.assertEqual(ledger['releases'][0]['repo'], 'https://example.invalid/fedora/releases/v1.0.0')


class Repository(unittest.TestCase):
    def test_rejects_injected_or_insecure_base_url(self):
        for base in ('http://example.invalid', 'https://a/\n[evil]', 'https://a/?x=y', 'https://user@a/'):
            with self.assertRaises(ValueError):
                repository.validate_base(base)

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
                        repository.configure('https://example.invalid', 'a' * 40, 'b' * 40, 'testing', root)
            self.assertFalse((root / 'etc/yum.repos.d/ryoku.repo').exists())


if __name__ == '__main__':
    unittest.main()
