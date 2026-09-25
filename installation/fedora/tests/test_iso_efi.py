"""Exercise EFI verification with real ISO and FAT filesystems, without root."""

from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest


VERIFY = Path(__file__).resolve().parents[1] / "verify-iso-efi.sh"
TOOLS = ("xorriso", "mkfs.fat", "mmd", "mcopy")
CONFIG = (
    "search --no-floppy --set=root -l Ryoku-44-x86_64\n"
    "linux /images/pxeboot/vmlinuz inst.ks=hd:LABEL=Ryoku-44-x86_64:/ryoku.ks\n"
    "initrd /images/pxeboot/initrd.img\n"
)


@unittest.skipUnless(all(shutil.which(tool) for tool in TOOLS),
                     "requires xorriso, dosfstools and mtools")
class TestIsoEfi(unittest.TestCase):
    def verify_fixture(self, embedded=CONFIG, outer=CONFIG, kernel=True):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            tree = root / "tree"
            boot = tree / "EFI/BOOT"
            pxeboot = tree / "images/pxeboot"
            boot.mkdir(parents=True)
            pxeboot.mkdir(parents=True)
            (boot / "grub.cfg").write_text(outer)
            if kernel:
                (pxeboot / "vmlinuz").write_bytes(b"kernel fixture")
            (pxeboot / "initrd.img").write_bytes(b"initrd fixture")
            image = root / "efiboot.img"
            with image.open("wb") as stream:
                stream.truncate(4 * 1024 * 1024)
            config = root / "embedded-grub.cfg"
            config.write_text(embedded)
            iso = root / "fixture.iso"
            for command in (
                ["mkfs.fat", str(image)],
                ["mmd", "-i", str(image), "::/EFI", "::/EFI/BOOT"],
                ["mcopy", "-i", str(image), str(config), "::/EFI/BOOT/grub.cfg"],
                ["xorriso", "-as", "mkisofs", "-R", "-o", str(iso),
                 "-append_partition", "2", "0xef", str(image), "-appended_part_as_gpt",
                 "-e", "--interval:appended_partition_2:all::", "-no-emul-boot", str(tree)],
            ):
                subprocess.run(command, check=True, capture_output=True)
            return subprocess.run(["bash", str(VERIFY), str(iso)],
                                  capture_output=True, text=True)

    def test_matching_efi_configuration(self):
        result = self.verify_fixture()
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_rejects_stale_embedded_volume_label(self):
        result = self.verify_fixture(embedded=CONFIG.replace("Ryoku-44-x86_64", "Fedora-44"))
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("rebuild without --skip-mkefiboot", result.stderr)

    def test_rejects_missing_installer_kernel(self):
        result = self.verify_fixture(kernel=False)
        self.assertNotEqual(result.returncode, 0)

    def test_rejects_missing_kickstart(self):
        config = CONFIG.replace("inst.ks=", "unused=")
        result = self.verify_fixture(embedded=config, outer=config)
        self.assertNotEqual(result.returncode, 0)


if __name__ == "__main__":
    unittest.main()
