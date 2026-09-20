#!/usr/bin/env python3
"""Unit tests for Fedora 44 Anaconda Kickstart recipe and ISO compose pipeline."""

import json
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

    def test_accounts_are_locked_without_embedded_passwords(self):
        """Accounts: ryoku and root are pre-created locked; no embedded credentials."""
        self.assertIn("rootpw --lock", self.content, "Root password must be locked")
        has_locked_user = any("user" in line and "--name=ryoku" in line and "--lock" in line for line in self.lines)
        self.assertTrue(has_locked_user, "ryoku user must be created locked")
        self.assertNotIn("--plaintext", self.content, "Kickstart must never contain plaintext passwords")

    def test_packages_section_and_exclusions(self):
        """Packages payload: Contains core packages from packages.list and excludes ffmpeg-free."""
        self.assertIn("%packages", self.content)
        self.assertIn("%end", self.content)

        # Ensure required core packages are listed
        for pkg in ["kernel", "btrfs-progs", "chromium", "tmux", "neovim", "fish", "firewalld", "NetworkManager", "sddm", "ffmpeg", "niri", "ryoku-desktop"]:
            self.assertIn(pkg, self.content, f"Kickstart %packages missing required package: {pkg}")

        # Ensure ffmpeg-free packages are excluded
        for free_lib in ["-ffmpeg-free", "-libavcodec-free", "-libavdevice-free", "-libavfilter-free", "-libavformat-free", "-libavutil-free"]:
            self.assertIn(free_lib, self.content, f"Kickstart %packages must exclude: {free_lib}")

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
