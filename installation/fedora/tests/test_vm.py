#!/usr/bin/env python3
"""Unit tests for Fedora 44 UEFI VM test harness and validation components."""

import json
import os
from pathlib import Path
import tempfile
import unittest

REPO_ROOT = Path(__file__).resolve().parents[3]
HARNESS_PY = REPO_ROOT / "installation" / "tests" / "fedora-iso-vm.py"
BASE_KS_PATH = REPO_ROOT / "installation" / "fedora" / "kickstart" / "ryoku.ks"

sys_path_added = str(REPO_ROOT / "installation" / "tests")
if sys_path_added not in os.sys.path:
    os.sys.path.insert(0, sys_path_added)

import importlib.util
spec = importlib.util.spec_from_file_location("fedora_iso_vm", str(HARNESS_PY))
fedora_iso_vm = importlib.util.module_from_spec(spec)
spec.loader.exec_module(fedora_iso_vm)

OVMFLocator = fedora_iso_vm.OVMFLocator
KickstartGenerator = fedora_iso_vm.KickstartGenerator
QemuCommandBuilder = fedora_iso_vm.QemuCommandBuilder
FirstBootPrompter = fedora_iso_vm.FirstBootPrompter
SELinuxVerifier = fedora_iso_vm.SELinuxVerifier
SDDMGateVerifier = fedora_iso_vm.SDDMGateVerifier
VMHarness = fedora_iso_vm.VMHarness


class TestOVMFDetection(unittest.TestCase):
    """Test OVMF firmware detection and vars preparation."""

    def test_ovmf_locator_explicit_paths(self):
        with tempfile.TemporaryDirectory() as tmp:
            code_file = Path(tmp) / "test_code.fd"
            vars_file = Path(tmp) / "test_vars.fd"
            code_file.write_bytes(b"CODE")
            vars_file.write_bytes(b"VARS")

            locator = OVMFLocator(str(code_file), str(vars_file))
            self.assertTrue(locator.is_available())
            self.assertEqual(locator.code_path, str(code_file))
            self.assertEqual(locator.vars_path, str(vars_file))

            dest_dir = Path(tmp) / "dest"
            dest_dir.mkdir()
            copied = locator.prepare_vars_copy(dest_dir)
            self.assertTrue(copied.is_file())
            self.assertEqual(copied.read_bytes(), b"VARS")

    def test_ovmf_locator_missing_files(self):
        locator = OVMFLocator("/nonexistent/code.fd", "/nonexistent/vars.fd")
        self.assertFalse(locator.is_available())
        with tempfile.TemporaryDirectory() as tmp:
            with self.assertRaises(FileNotFoundError):
                locator.prepare_vars_copy(Path(tmp))

    def test_ovmf_locator_secure_boot(self):
        with tempfile.TemporaryDirectory() as tmp:
            code_file = Path(tmp) / "OVMF_CODE.secboot.fd"
            vars_file = Path(tmp) / "OVMF_VARS.secboot.fd"
            code_file.write_bytes(b"SECBOOT_CODE")
            vars_file.write_bytes(b"SECBOOT_VARS")

            locator = OVMFLocator(str(code_file), str(vars_file), secure_boot=True)
            self.assertTrue(locator.is_available())
            self.assertTrue(locator.secure_boot)
            self.assertEqual(locator.code_path, str(code_file))
            self.assertEqual(locator.vars_path, str(vars_file))

            dest_dir = Path(tmp) / "dest"
            dest_dir.mkdir()
            copied = locator.prepare_vars_copy(dest_dir)
            self.assertTrue(copied.is_file())
            self.assertEqual(copied.read_bytes(), b"SECBOOT_VARS")
            self.assertEqual(copied.name, "OVMF_VARS.fd")


class TestKickstartVariants(unittest.TestCase):
    """Test automated Kickstart generation for unencrypted and LUKS2 paths."""

    def setUp(self):
        self.generator = KickstartGenerator(BASE_KS_PATH)

    def test_kickstart_variant_unencrypted(self):
        ks = self.generator.generate(target_disk="vda", encrypted=False)

        # Check target drive safety adaptation
        self.assertNotIn("clearpart --none", ks)
        self.assertIn("ignoredisk --only-use=vda", ks)
        self.assertIn("clearpart --all --initlabel --drives=vda", ks)

        # Check base partitioning
        self.assertIn("part /boot/efi --fstype=\"efi\" --size=600", ks)
        self.assertIn("part /boot --fstype=\"ext4\" --size=2048", ks)
        self.assertIn("part btrfs.01 --fstype=\"btrfs\" --size=1024 --grow", ks)
        self.assertIn("btrfs / --subvol --name=root btrfs.01", ks)
        self.assertIn("btrfs /home --subvol --name=home btrfs.01", ks)

        # Check encryption absence
        self.assertNotIn("--encrypted", ks)
        self.assertNotIn("--luks-version", ks)

        # Check offline provisioner post script
        self.assertIn("%post --nochroot --erroronfail", ks)
        self.assertIn("provision-target.py", ks)

    def test_kickstart_variant_luks2_encrypted(self):
        passphrase = "SecretPassword456"
        ks = self.generator.generate(target_disk="vda", encrypted=True, passphrase=passphrase)

        # Check target drive adaptation
        self.assertIn("ignoredisk --only-use=vda", ks)
        self.assertIn("clearpart --all --initlabel --drives=vda", ks)

        # Check LUKS2 encryption parameters
        self.assertIn("--encrypted", ks)
        self.assertIn("--luks-version=luks2", ks)
        self.assertIn(f"--passphrase=\"{passphrase}\"", ks)

        # Subvolumes must still mount from btrfs.01
        self.assertIn("btrfs / --subvol --name=root btrfs.01", ks)
        self.assertIn("btrfs /home --subvol --name=home btrfs.01", ks)


class TestQemuCommandAssembly(unittest.TestCase):
    """Test QEMU command generation and disconnected network enforcement."""

    def test_disconnected_network_enforced(self):
        builder = QemuCommandBuilder(
            ovmf_code="/fake/code.fd",
            ovmf_vars="/fake/vars.fd",
            target_disk="/fake/target.qcow2",
            kvm_available=False,
        )

        install_cmd = builder.build(boot_from_cdrom=True)
        boot_cmd = builder.build(boot_from_cdrom=False)

        # Assert strictly disconnected network
        self.assertIn("-nic", install_cmd)
        nic_idx = install_cmd.index("-nic")
        self.assertEqual(install_cmd[nic_idx + 1], "none")

        self.assertIn("-nic", boot_cmd)
        nic_idx_boot = boot_cmd.index("-nic")
        self.assertEqual(boot_cmd[nic_idx_boot + 1], "none")

    def test_uefi_and_machine_architecture(self):
        builder = QemuCommandBuilder(
            ovmf_code="/fake/code.fd",
            ovmf_vars="/fake/vars.fd",
            target_disk="/fake/target.qcow2",
            memory_mb=4096,
            smp=4,
            iso_path="/fake/ryoku.iso",
            kvm_available=True,
        )

        cmd = builder.build(boot_from_cdrom=True)
        self.assertEqual(cmd[0], "qemu-system-x86_64")
        self.assertIn("-machine", cmd)
        self.assertEqual(cmd[cmd.index("-machine") + 1], "q35")
        self.assertIn("-enable-kvm", cmd)
        self.assertIn("-cpu", cmd)
        self.assertEqual(cmd[cmd.index("-cpu") + 1], "host")
        self.assertIn("-boot", cmd)
        self.assertEqual(cmd[cmd.index("-boot") + 1], "d")

    def test_secure_boot_command_assembly(self):
        builder = QemuCommandBuilder(
            ovmf_code="/usr/share/OVMF/OVMF_CODE.secboot.fd",
            ovmf_vars="/tmp/test/OVMF_VARS.secboot.fd",
            target_disk="/fake/target.qcow2",
            memory_mb=4096,
            smp=4,
            iso_path="/fake/ryoku.iso",
            kvm_available=True,
            secure_boot=True,
        )

        cmd = builder.build(boot_from_cdrom=True)
        self.assertEqual(cmd[0], "qemu-system-x86_64")
        self.assertIn("-machine", cmd)
        self.assertEqual(cmd[cmd.index("-machine") + 1], "q35,smm=on")
        self.assertIn("-enable-kvm", cmd)
        self.assertIn("-cpu", cmd)
        self.assertEqual(cmd[cmd.index("-cpu") + 1], "host")

        # Assert secure flash parameters
        self.assertIn("-global", cmd)
        global_indices = [i for i, x in enumerate(cmd) if x == "-global"]
        global_vals = [cmd[i + 1] for i in global_indices]
        self.assertIn("driver=cfi.pflash01,property=secure,value=on", global_vals)
        self.assertIn("ICH9-LPC.disable_s3=1", global_vals)

        # Assert pflash drive lines
        drive_indices = [i for i, x in enumerate(cmd) if x == "-drive"]
        drive_vals = [cmd[i + 1] for i in drive_indices]
        self.assertTrue(any("OVMF_CODE.secboot.fd" in d and "readonly=on" in d for d in drive_vals))
        self.assertTrue(any("OVMF_VARS.secboot.fd" in d for d in drive_vals))


class TestFirstBootPrompter(unittest.TestCase):
    """Test interactive first-boot console sequence model."""

    def test_prompt_sequence_completeness(self):
        prompts = FirstBootPrompter.get_prompts_and_answers("rootpw123", "userpw123")
        self.assertEqual(len(prompts), 10)

        steps = [step for step, _, _ in prompts]
        expected_steps = [
            "locale", "msg_locale", "keymap", "timezone", "hostname",
            "root_pw", "root_pw_confirm", "user_prompt", "user_pw", "user_pw_confirm",
        ]
        self.assertEqual(steps, expected_steps)

        # Check answers
        answers_dict = {step: ans for step, _, ans in prompts}
        self.assertEqual(answers_dict["locale"], "en_US.UTF-8")
        self.assertEqual(answers_dict["keymap"], "us")
        self.assertEqual(answers_dict["root_pw"], "rootpw123")
        self.assertEqual(answers_dict["user_pw"], "userpw123")


class TestSELinuxAndSDDMVerifiers(unittest.TestCase):
    """Test SELinux AVC parser and SDDM gate verifier."""

    def test_selinux_clean_log(self):
        clean_log = (
            "type=PROCTITLE msg=audit(1695200000.000:10): proctitle=\"niri\"\n"
            "type=SYSCALL msg=audit(1695200000.000:10): arch=c000003e success=yes exit=0\n"
        )
        self.assertTrue(SELinuxVerifier.verify_clean(clean_log))
        count, denials = SELinuxVerifier.parse_ausearch_output(clean_log)
        self.assertEqual(count, 0)
        self.assertEqual(denials, [])

    def test_selinux_denial_detected(self):
        denied_log = (
            "type=AVC msg=audit(1695200000.000:20): avc:  denied  { read } for  "
            "pid=123 comm=\"quickshell\" name=\"test\" dev=\"vda3\" ino=456 scontext=system_u:system_r:user_t:s0 tcontext=system_u:object_r:etc_t:s0\n"
        )
        self.assertFalse(SELinuxVerifier.verify_clean(denied_log))
        count, denials = SELinuxVerifier.parse_ausearch_output(denied_log)
        self.assertEqual(count, 1)
        self.assertIn("avc:  denied", denials[0])

    def test_sddm_gate_precondition(self):
        self.assertFalse(SDDMGateVerifier.check_gate_precondition(complete_file_exists=False))
        self.assertTrue(SDDMGateVerifier.check_gate_precondition(complete_file_exists=True))


class TestVMHarnessStaging(unittest.TestCase):
    """Test VM harness staging and provenance creation."""

    def test_stage_environment_and_provenance(self):
        with tempfile.TemporaryDirectory() as tmp:
            work_dir = Path(tmp)
            harness = VMHarness(
                iso_path=str(work_dir / "test.iso"),
                work_dir=work_dir,
                encrypted=True,
                dry_run=True,
            )

            harness.stage_environment()
            self.assertTrue((work_dir / "ks.cfg").is_file())
            ks_content = (work_dir / "ks.cfg").read_text(encoding="utf-8")
            self.assertIn("--luks-version=luks2", ks_content)

            harness.record_provenance(status="staged")
            self.assertTrue((work_dir / "vm-provenance.json").is_file())

            prov = json.loads((work_dir / "vm-provenance.json").read_text(encoding="utf-8"))
            self.assertEqual(prov["harness"], "ryoku-fedora-iso-vm")
            self.assertEqual(prov["status"], "staged")
            self.assertTrue(prov["encrypted"])
            self.assertIn("-nic none", prov["network_policy"])

    def test_stage_environment_and_provenance_secure_boot(self):
        with tempfile.TemporaryDirectory() as tmp:
            work_dir = Path(tmp)
            harness = VMHarness(
                iso_path=str(work_dir / "test.iso"),
                work_dir=work_dir,
                encrypted=False,
                secure_boot=True,
                dry_run=True,
            )

            harness.stage_environment()
            harness.record_provenance(status="staged")

            prov_file = work_dir / "vm-provenance.json"
            self.assertTrue(prov_file.is_file())
            prov = json.loads(prov_file.read_text(encoding="utf-8"))
            self.assertTrue(prov["secure_boot"])
            self.assertTrue(prov["ovmf"]["secure_boot"])
            self.assertIn("Secure Boot enabled validation", prov["verification_checks"])


if __name__ == "__main__":
    unittest.main()
