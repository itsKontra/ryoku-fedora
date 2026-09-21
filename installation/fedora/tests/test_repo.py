#!/usr/bin/env python3
"""Unit tests for Fedora 44 offline repository builder and dependency closure resolver."""

import json
from pathlib import Path
import subprocess
import tempfile
import unittest
import xml.etree.ElementTree as ET
from unittest.mock import MagicMock, patch

import importlib.util

import sys

REPO_ROOT = Path(__file__).resolve().parents[3]
BUILD_REPO_PATH = REPO_ROOT / "installation" / "fedora" / "build-repo.py"

spec = importlib.util.spec_from_file_location("build_repo", BUILD_REPO_PATH)
build_repo = importlib.util.module_from_spec(spec)
sys.modules["build_repo"] = build_repo
spec.loader.exec_module(build_repo)


class TestPackageListParsing(unittest.TestCase):
    def test_read_packages_list_valid(self):
        with tempfile.NamedTemporaryFile("w", suffix=".list", delete=False) as f:
            f.write("""# Sample package list
[base]
kernel
systemd
# A comment in section
glibc

[utilities]
bash
systemd # Duplicate should be deduplicated
fish
""")
            path = Path(f.name)

        try:
            packages = build_repo.read_packages_list(path)
            self.assertEqual(packages, ["kernel", "systemd", "glibc", "bash", "fish"])
        finally:
            path.unlink()

    def test_read_packages_list_section_filter(self):
        with tempfile.NamedTemporaryFile("w", suffix=".list", delete=False) as f:
            f.write("""[base]
kernel
systemd

[utilities]
tmux
bash
""")
            path = Path(f.name)

        try:
            base_pkgs = build_repo.read_packages_list(path, selected_sections=["base"])
            self.assertEqual(base_pkgs, ["kernel", "systemd"])

            util_pkgs = build_repo.read_packages_list(path, selected_sections=["utilities"])
            self.assertEqual(util_pkgs, ["tmux", "bash"])
        finally:
            path.unlink()

    def test_read_packages_list_missing_file(self):
        with self.assertRaises(FileNotFoundError):
            build_repo.read_packages_list(Path("/nonexistent/path/packages.list"))

    def test_official_packages_list(self):
        official_path = REPO_ROOT / "installation" / "fedora" / "packages.list"
        self.assertTrue(official_path.is_file())
        packages = build_repo.read_packages_list(official_path)
        self.assertGreater(len(packages), 50)
        # Required core packages from Milestone 2 plan
        for required in ["kernel", "chromium", "tmux", "neovim", "fish", "firewalld", "NetworkManager", "ffmpeg-free"]:
            self.assertIn(required, packages, f"Missing required package {required}")


class TestKeyVerification(unittest.TestCase):
    @patch("subprocess.check_output")
    def test_extract_key_fingerprint(self, mock_check_output):
        mock_check_output.return_value = """tru::1:1750000000:0:3:1:5
pub:u:4096:1:1234567890ABCDEF:1750000000:::u:::scESC:
fpr:::::::::36F612DCF27F7D1A48A835E4DBFCF71C6D9F90A6:
uid:u::::1750000000::FEDORA44::Test <test@example.com>:
"""
        with tempfile.NamedTemporaryFile(suffix=".asc", delete=False) as f:
            f.write(b"mock key content")
            path = Path(f.name)

        try:
            fp = build_repo.extract_key_fingerprint(path)
            self.assertEqual(fp, "36F612DCF27F7D1A48A835E4DBFCF71C6D9F90A6")
        finally:
            path.unlink()

    @patch("build_repo.extract_key_fingerprint")
    def test_verify_key_success_and_mismatch(self, mock_extract):
        mock_extract.return_value = "36F612DCF27F7D1A48A835E4DBFCF71C6D9F90A6"
        sample_path = Path("/sample/key.asc")

        # Success case with formatted spaces
        res = build_repo.verify_key(sample_path, "36F6 12DC F27F 7D1A 48A8 35E4 DBFC F71C 6D9F 90A6")
        self.assertEqual(res, "36F612DCF27F7D1A48A835E4DBFCF71C6D9F90A6")

        # Mismatch case
        with self.assertRaises(ValueError):
            build_repo.verify_key(sample_path, "0000000000000000000000000000000000000000")

    def test_verify_all_pinned_keys_against_repo(self):
        keys_dir = REPO_ROOT / "installation" / "fedora" / "keys"
        self.assertTrue(keys_dir.is_dir())
        verified = build_repo.verify_all_pinned_keys(keys_dir)
        for key_name in build_repo.PINNED_KEYS:
            self.assertIn(key_name, verified)
            self.assertEqual(verified[key_name], build_repo.PINNED_KEYS[key_name])


class TestDnfArgsAndFFmpegValidation(unittest.TestCase):
    def test_build_dnf_resolve_args(self):
        args = build_repo.build_dnf_resolve_args(
            packages=["kernel", "ffmpeg"],
            installroot="/mnt/sysroot",
            exclude=["ffmpeg-free"],
            repofrompath=["offline,file:///repo"],
            disablerepo="*",
            install_weak_deps=False,
            assumeno=True,
            destdir="/repo/rpms",
        )
        self.assertIn("--installroot=/mnt/sysroot", args)
        self.assertIn("--setopt=install_weak_deps=False", args)
        self.assertIn("--exclude=ffmpeg-free", args)
        self.assertIn("--disablerepo=*", args)
        self.assertIn("--repofrompath=offline,file:///repo", args)
        self.assertIn("--assumeno", args)
        self.assertIn("--destdir=/repo/rpms", args)
        self.assertEqual(args[-2:], ["kernel", "ffmpeg"])

    def test_validate_ffmpeg_transaction_success(self):
        sample_output = """
Paket                    Architektur Version         Paketquelle
Wird installiert:
 ffmpeg                  x86_64      8.1.2-3.fc44    rpmfusion-free-updates
 ffmpeg-libs             x86_64      8.1.2-3.fc44    rpmfusion-free-updates
 libavdevice             x86_64      8.1.2-3.fc44    rpmfusion-free-updates
 x264-libs               x86_64      0.165-5.fc44    rpmfusion-free
"""
        self.assertTrue(build_repo.validate_ffmpeg_transaction(sample_output))

    def test_validate_ffmpeg_transaction_forbidden_free(self):
        sample_output = """
Wird installiert:
 ffmpeg-free             x86_64      8.1.2-4.fc44    updates
 libavcodec-free         x86_64      8.1.2-4.fc44    updates
 ffmpeg                  x86_64      8.1.2-3.fc44    rpmfusion-free
"""
        with self.assertRaises(RuntimeError) as ctx:
            build_repo.validate_ffmpeg_transaction(sample_output)
        self.assertIn("forbidden ffmpeg-free", str(ctx.exception))

    def test_validate_ffmpeg_transaction_missing_ffmpeg(self):
        sample_output = """
Wird installiert:
 bash                    x86_64      5.3.9-3.fc44    updates
"""
        with self.assertRaises(RuntimeError) as ctx:
            build_repo.validate_ffmpeg_transaction(sample_output)
        self.assertIn("full RPM Fusion ffmpeg package", str(ctx.exception))


class TestManifestAndRepoCreation(unittest.TestCase):
    def test_generate_manifest(self):
        with tempfile.TemporaryDirectory() as td:
            repo_dir = Path(td)
            sample_pkg = repo_dir / "test-pkg-1.0-1.fc44.x86_64.rpm"
            sample_pkg.write_bytes(b"rpm package content binary payload")

            manifest_path = repo_dir / "manifest.json"
            manifest = build_repo.generate_manifest(repo_dir, manifest_path)

            self.assertEqual(manifest["total_packages"], 1)
            self.assertIn(sample_pkg.name, manifest["packages"])
            self.assertIn(sample_pkg.name, manifest["sha256"])
            self.assertTrue(manifest_path.is_file())

            loaded = json.loads(manifest_path.read_text(encoding="utf-8"))
            self.assertEqual(loaded["total_packages"], 1)

    @patch("subprocess.check_call")
    def test_create_repo(self, mock_call):
        with tempfile.TemporaryDirectory() as td:
            build_repo.create_repo(td)
            mock_call.assert_called_once_with([
                "createrepo_c", "--update", "--checksum=sha256", "--groupfile",
                str(Path(td) / "comps.xml"), td,
            ])
            comps = ET.parse(Path(td) / "comps.xml").getroot()
            self.assertEqual(comps.findtext("environment/id"), "ryoku-desktop-environment")
            self.assertEqual(comps.findtext("environment/name"), "Ryoku Desktop")
            self.assertEqual([g.text for g in comps.findall("environment/grouplist/groupid")],
                             ["core", "ryoku-desktop"])
            packages = comps.findall("group/packagelist/packagereq")
            self.assertTrue(all(p.attrib["type"] == "mandatory" for p in packages))
            self.assertEqual([p.text for p in packages], build_repo.read_packages_list(
                BUILD_REPO_PATH.parent / "packages.list"))

    def test_verify_offline_closure_missing_repomd(self):
        with tempfile.TemporaryDirectory() as td:
            with self.assertRaises(FileNotFoundError):
                build_repo.verify_offline_closure(td, ["kernel"])

    @patch("subprocess.run")
    def test_verify_offline_closure_success(self, mock_run):
        with tempfile.TemporaryDirectory() as td:
            repodata = Path(td) / "repodata"
            repodata.mkdir(parents=True)
            (repodata / "repomd.xml").write_text("<repomd/>")

            mock_res = MagicMock()
            mock_res.stdout = "Transaktionszusammenfassung:\nVorgang vom Benutzer abgebrochen."
            mock_res.stderr = ""
            mock_res.returncode = 1
            mock_run.return_value = mock_res

            self.assertTrue(build_repo.verify_offline_closure(td, ["kernel"]))

    @patch("subprocess.run")
    def test_verify_offline_closure_failure(self, mock_run):
        with tempfile.TemporaryDirectory() as td:
            repodata = Path(td) / "repodata"
            repodata.mkdir(parents=True)
            (repodata / "repomd.xml").write_text("<repomd/>")

            mock_res = MagicMock()
            mock_res.stdout = "Failed to resolve transaction: missing dependency foo"
            mock_res.stderr = ""
            mock_res.returncode = 1
            mock_run.return_value = mock_res

            with self.assertRaises(RuntimeError) as ctx:
                build_repo.verify_offline_closure(td, ["kernel"])
            self.assertIn("Offline dependency closure failed", str(ctx.exception))


if __name__ == "__main__":
    unittest.main()
