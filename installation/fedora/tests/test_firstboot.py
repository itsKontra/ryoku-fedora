import importlib.util
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch


SOURCE = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(SOURCE))
import firstboot

spec = importlib.util.spec_from_file_location("prepare", SOURCE / "prepare-firstboot.py")
prepare = importlib.util.module_from_spec(spec)
spec.loader.exec_module(prepare)


class FirstBootTest(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        self.write("etc/os-release", 'ID=fedora\nVERSION_ID="44"\n')
        self.write("etc/passwd", "root:x:0:0:root:/root:/bin/bash\nryoku:x:1000:1000::/home/ryoku:/usr/bin/fish\n")
        self.write("etc/group", "wheel:x:10:ryoku\n")
        self.write("etc/shadow", "root:!::0:99999:7:::\nryoku:!::0:99999:7:::\n")
        self.write("etc/locale.conf", "LANG=en_US.UTF-8\n")
        self.write("etc/vconsole.conf", "KEYMAP=us\n")
        self.write("etc/hostname", "compose-host\n")
        self.write("etc/machine-id", "0123456789abcdef0123456789abcdef\n")
        self.write("var/lib/dbus/machine-id", "0123456789abcdef0123456789abcdef\n")
        (self.root / "etc/localtime").symlink_to("../usr/share/zoneinfo/UTC")
        for name in ("python3", "systemd-firstboot", "passwd", "fish"):
            self.write(f"usr/bin/{name}", "test fixture\n")
        self.sync = patch.object(firstboot.os, "sync")
        self.sync.start()
        self.addCleanup(self.sync.stop)
        self.calls = []

    def write(self, relative, value):
        path = self.root / relative
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(value)

    def answer(self, command, **kwargs):
        self.calls.append(command)
        for name, relative in firstboot.SETTINGS:
            if f"--prompt-{name}" in command:
                if name == "timezone":
                    (self.root / relative).symlink_to("../usr/share/zoneinfo/Europe/Vienna")
                else:
                    self.write(relative, {"locale": "LANG=de_AT.UTF-8", "keymap": "KEYMAP=de", "hostname": "chosen-host"}[name])
                return
        user = "root" if "--prompt-root-password" in command else "ryoku"
        path = self.root / "etc/shadow"
        path.write_text(path.read_text().replace(f"{user}:!:", f"{user}:$y$chosen-{user}:"))

    def test_prepare_discards_seeded_answers_and_identity_once(self):
        prepare.prepare(self.root)
        for _, relative in firstboot.SETTINGS:
            self.assertFalse((self.root / relative).is_symlink())
            self.assertFalse((self.root / relative).exists())
        self.assertEqual((self.root / "etc/machine-id").read_text(), "uninitialized\n")
        self.assertEqual((self.root / "var/lib/dbus/machine-id").readlink(), Path("/etc/machine-id"))
        self.assertEqual((self.root / "etc/systemd/system/systemd-firstboot.service").readlink(), Path("/dev/null"))
        self.assertTrue((self.root / "usr/libexec/ryoku-firstboot").stat().st_mode & 0o111)
        self.write("etc/hostname", "chosen-host")
        prepare.prepare(self.root)
        self.assertEqual((self.root / "etc/hostname").read_text(), "chosen-host")

    def test_complete_requires_all_six_answers_and_never_repeats(self):
        prepare.prepare(self.root)
        with patch.dict("os.environ", {"CREDENTIALS_DIRECTORY": "/some/credentials"}):
            def answer(command, **kwargs):
                self.assertNotIn("CREDENTIALS_DIRECTORY", kwargs["env"])
                self.answer(command, **kwargs)
            firstboot.setup(self.root, answer)
        self.assertEqual(len(self.calls), 6)
        self.assertTrue((self.root / firstboot.STATE / "complete").exists())
        firstboot.setup(self.root, lambda *a, **kw: self.fail("repeated setup"))

    def test_failure_at_every_step_resumes_without_resetting_answers(self):
        for failure in range(6):
            with self.subTest(step=failure):
                self.calls = []
                prepare.prepare(self.root)
                def interrupted(command, **kwargs):
                    if len(self.calls) == failure:
                        raise subprocess.CalledProcessError(1, command)
                    self.answer(command, **kwargs)
                with self.assertRaises(subprocess.CalledProcessError):
                    firstboot.setup(self.root, interrupted)
                self.assertFalse((self.root / firstboot.STATE / "complete").exists())
                before = (self.root / "etc/shadow").read_text()
                firstboot.setup(self.root, self.answer)
                self.assertEqual(len(self.calls), 6)
                if failure == 5:
                    self.assertEqual(before.splitlines()[0], (self.root / "etc/shadow").read_text().splitlines()[0])
                for _, relative in firstboot.SETTINGS:
                    (self.root / relative).unlink()
                self.write("etc/shadow", "root:!::0:99999:7:::\nryoku:!::0:99999:7:::\n")
                (self.root / firstboot.STATE / "complete").unlink()

    def test_crash_after_password_write_before_completion_preserves_both(self):
        prepare.prepare(self.root)
        def interrupted(command, **kwargs):
            self.answer(command, **kwargs)
            if command[0] == "passwd":
                raise KeyboardInterrupt
        with self.assertRaises(KeyboardInterrupt):
            firstboot.setup(self.root, interrupted)
        before = (self.root / "etc/shadow").read_bytes()
        firstboot.setup(self.root, lambda *a, **kw: self.fail("password reset"))
        self.assertEqual(before, (self.root / "etc/shadow").read_bytes())

    def test_skipped_answer_cannot_complete_setup(self):
        prepare.prepare(self.root)
        skipped = False
        def answer(command, **kwargs):
            nonlocal skipped
            if not skipped:
                skipped = True
                return
            self.answer(command, **kwargs)
        firstboot.setup(self.root, answer)
        self.assertTrue(skipped)
        self.assertEqual(len(self.calls), 6)

    def test_unarmed_target_fails_closed(self):
        with self.assertRaisesRegex(ValueError, "not armed"):
            firstboot.setup(self.root, self.answer)
        self.assertFalse(self.calls)

    def test_skipping_root_password_reprompts(self):
        prepare.prepare(self.root)
        skipped = False
        def answer(command, **kwargs):
            nonlocal skipped
            if "--prompt-root-password" in command and not skipped:
                skipped = True
                return
            self.answer(command, **kwargs)
        firstboot.setup(self.root, answer)
        self.assertTrue(skipped)
        self.assertTrue(firstboot.password_set(self.root, "root"))

    def test_prepare_rejects_passwordless_account(self):
        self.write("etc/shadow", "root::0:0:99999:7:::\nryoku:!::0:99999:7:::\n")
        with self.assertRaisesRegex(ValueError, "locked, not passwordless"):
            prepare.prepare(self.root)

    def test_prepare_rejects_locked_existing_password(self):
        self.write("etc/shadow", "root:!$y$existing:0:0:99999:7:::\nryoku:!::0:99999:7:::\n")
        with self.assertRaisesRegex(ValueError, "existing locked root password"):
            prepare.prepare(self.root)

    def test_prepare_rejects_existing_password_and_wrong_target(self):
        self.write("etc/shadow", "root:$y$existing::0:99999:7:::\nryoku:!::0:99999:7:::\n")
        with self.assertRaisesRegex(ValueError, "existing root password"):
            prepare.prepare(self.root)
        self.assertEqual((self.root / "etc/hostname").read_text(), "compose-host\n")
        with self.assertRaisesRegex(ValueError, "running system"):
            prepare.prepare(Path("/"))


if __name__ == "__main__":
    unittest.main()
