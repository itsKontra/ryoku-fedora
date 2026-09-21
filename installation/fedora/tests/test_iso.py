#!/usr/bin/env python3
"""Unit tests for Fedora 44 Anaconda Kickstart recipe and ISO compose pipeline."""

import json
import os
import shutil
from pathlib import Path
import subprocess
import tempfile
import unittest

REPO_ROOT = Path(__file__).resolve().parents[3]
KICKSTART_PATH = REPO_ROOT / "installation" / "fedora" / "kickstart" / "ryoku.ks"
BUILD_ISO_PATH = REPO_ROOT / "installation" / "fedora" / "build-iso.sh"
PACKAGES_LIST_PATH = REPO_ROOT / "installation" / "fedora" / "packages.list"


class TestKickstartSpecification(unittest.TestCase):
    """Validate Anaconda Kickstart file against Ryoku ISO requirements."""

    def setUp(self):
        self.assertTrue(KICKSTART_PATH.is_file(), f"Kickstart file not found: {KICKSTART_PATH}")
        self.content = KICKSTART_PATH.read_text(encoding="utf-8")
        self.lines = [line.strip() for line in self.content.splitlines() if line.strip() and not line.strip().startswith("#")]

    def test_target_safety_forbids_clearpart_all(self):
        """Target safety: Forbid unattended whole-disk erasure; require user confirmation."""
        # clearpart --all is strictly forbidden on public installation media
        self.assertNotIn("clearpart --all", self.content, "Kickstart must NOT contain unattended 'clearpart --all'")
        self.assertIn("clearpart --none", self.content, "Kickstart must specify 'clearpart --none' to prevent auto-wiping")
        # No hardcoded target disks
        self.assertNotIn("ignoredisk --only-use", self.content, "Kickstart must not embed hardcoded ignoredisk drive picks")

    def test_partitioning_scheme_btrfs_and_uefi(self):
        """Partitioning scheme: 600 MiB ESP, 2 GiB /boot, Btrfs with root and home subvolumes."""
        # 600 MiB FAT32 ESP
        has_esp = any("part /boot/efi" in line and "--size=600" in line for line in self.lines)
        self.assertTrue(has_esp, "Missing 600 MiB EFI system partition at /boot/efi")

        # 2 GiB ext4 /boot
        has_boot = any("part /boot" in line and "--size=2048" in line and "ext4" in line for line in self.lines)
        self.assertTrue(has_boot, "Missing 2 GiB ext4 dedicated boot partition at /boot")

        # Btrfs partition and subvolumes
        has_btrfs_part = any("part btrfs" in line and "--grow" in line for line in self.lines)
        self.assertTrue(has_btrfs_part, "Missing growing Btrfs base partition")

        has_root_subvol = any("btrfs /" in line and "--subvol" in line and "--name=root" in line for line in self.lines)
        self.assertTrue(has_root_subvol, "Missing Btrfs root subvolume mounted at /")

        has_home_subvol = any("btrfs /home" in line and "--subvol" in line and "--name=home" in line for line in self.lines)
        self.assertTrue(has_home_subvol, "Missing Btrfs home subvolume mounted at /home")

        # Fedora zram swap policy: No disk swap partition defined
        has_swap_part = any("part swap" in line for line in self.lines)
        self.assertFalse(has_swap_part, "Kickstart should not define a disk swap partition (Fedora uses zram)")

    def test_repository_configuration(self):
        """Repository: Points to local media with high priority (--cost=10)."""
        has_local_repo = any("repo" in line and "--cost=10" in line and "run/install/repo" in line for line in self.lines)
        self.assertTrue(has_local_repo, "Kickstart must define local media repository with --cost=10")

    def test_dependency_coprs_match_packaged_installer(self):
        dependency_file = REPO_ROOT / "release/rpm/dependency-coprs"
        for line in dependency_file.read_text().splitlines():
            repo = line.split("#", 1)[0].strip()
            if repo:
                self.assertIn(f"/results/{repo}/fedora-44-x86_64/", self.content)

    def test_accounts_are_collected_in_anaconda(self):
        self.assertIn("rootpw --lock", self.lines)
        self.assertFalse(any(line.startswith("user ") for line in self.lines))
        self.assertIn("/mnt/sysroot --anaconda", self.content)
        self.assertIn("keyboard us", self.lines)
        self.assertNotIn("--plaintext", self.content)

    def test_packages_section_and_multimedia(self):
        """The environment uses the codec stack required by published Ryoku RPMs."""
        self.assertIn("%packages", self.content)
        self.assertIn("%end", self.content)

        self.assertIn("@^ryoku-desktop-environment", self.lines)

        self.assertNotIn("-ffmpeg-free", self.lines)
        packages = PACKAGES_LIST_PATH.read_text().splitlines()
        self.assertIn("ffmpeg-free", packages)
        self.assertNotIn("ffmpeg", packages)
        self.assertNotIn("setfiles", packages)
        self.assertNotIn("mesa-va-drivers", packages)
        self.assertNotIn("mesa-vdpau-drivers", packages)

    def test_post_nochroot_executes_provisioner(self):
        """Post script: Executes offline desktop provisioner on /mnt/sysroot."""
        self.assertIn("%post --nochroot --erroronfail", self.content)
        self.assertIn("provision-target.py", self.content)
        self.assertIn("/mnt/sysroot", self.content)


class TestComposePipelineScript(unittest.TestCase):
    """Validate build-iso.sh command line interface and staging logic."""

    def test_build_iso_help(self):
        """Verify build-iso.sh --help outputs usage information."""
        res = subprocess.run([str(BUILD_ISO_PATH), "--help"], capture_output=True, text=True)
        self.assertEqual(res.returncode, 0)
        self.assertIn("Compose pipeline for the Fedora 44 Ryoku Installation ISO", res.stdout)
        self.assertIn("--ks", res.stdout)
        self.assertIn("--stage-only", res.stdout)

    def test_build_iso_missing_kickstart(self):
        """Verify build-iso.sh fails actionably when Kickstart file is missing."""
        res = subprocess.run([str(BUILD_ISO_PATH), "--ks", "/nonexistent/kickstart.ks"], capture_output=True, text=True)
        self.assertNotEqual(res.returncode, 0)
        self.assertIn("Kickstart file not found", res.stderr)

    def test_compose_rejects_missing_mkksiso_before_staging(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            tools = root / "bin"
            tools.mkdir()
            for name in ("bash", "dirname", "git", "date", "mkdir", "python3"):
                executable = shutil.which(name)
                if executable:
                    (tools / name).symlink_to(executable)
            result = subprocess.run([
                str(BUILD_ISO_PATH), "--work-dir", str(root / "work"),
                "--out-dir", str(root / "out"),
            ], env={**os.environ, "PATH": str(tools)}, capture_output=True, text=True)
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("mkksiso (lorax) is required", result.stderr)
            self.assertFalse((root / "work/iso_root").exists())
            self.assertFalse(list((root / "out").glob("*.iso")))

    def test_build_iso_stage_only_execution(self):
        """Verify build-iso.sh --stage-only prepares staging tree and writes provenance."""
        with tempfile.TemporaryDirectory() as temp_dir:
            work_dir = Path(temp_dir) / "work"
            out_dir = Path(temp_dir) / "out"
            repo_dir = Path(temp_dir) / "repo"
            repo_dir.mkdir()

            cmd = [
                str(BUILD_ISO_PATH),
                "--ks", str(KICKSTART_PATH),
                "--repo-dir", str(repo_dir),
                "--work-dir", str(work_dir),
                "--out-dir", str(out_dir),
                "--skip-key-verify",
                "--skip-closure-verify",
                "--stage-only",
            ]
            res = subprocess.run(cmd, capture_output=True, text=True)
            self.assertEqual(res.returncode, 0, f"build-iso.sh failed:\n{res.stderr}\n{res.stdout}")

            # Verify staged files
            stage_dir = work_dir / "iso_root"
            self.assertTrue((stage_dir / "ryoku.ks").is_file())
            self.assertTrue((stage_dir / "ks.cfg").is_file())
            self.assertTrue((stage_dir / "installation/fedora/provision-target.py").is_file())
            self.assertTrue((stage_dir / "installation/fedora/prepare-firstboot.py").is_file())
            self.assertTrue((stage_dir / "ryoku/assets/wallpapers").is_dir())
            self.assertTrue((stage_dir / "ryoku/assets/brand").is_dir())
            self.assertTrue((stage_dir / "ryoku/assets/ryodecors").is_dir())
            self.assertTrue((stage_dir / "ryoku/apps/npm/npmrc").is_file())
            self.assertTrue((stage_dir / ".ryoku-media").is_file())

            # Verify output files
            manifest_file = out_dir / "manifest.json"
            provenance_file = out_dir / "provenance.json"
            self.assertTrue(manifest_file.is_file())
            self.assertTrue(provenance_file.is_file())

            # Validate provenance contents
            prov = json.loads(provenance_file.read_text(encoding="utf-8"))
            self.assertEqual(prov["fedora_version"], "44")
            self.assertEqual(prov["arch"], "x86_64")
            self.assertTrue(prov["stage_only"])
            self.assertIn("source_commit", prov)
            self.assertIn("source_date_epoch", prov)
            self.assertEqual(prov["kickstart"]["path"], str(KICKSTART_PATH))
            self.assertGreater(len(prov["kickstart"]["sha256"]), 10)


if __name__ == "__main__":
    unittest.main()
