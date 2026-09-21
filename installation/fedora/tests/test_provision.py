import importlib.util
from pathlib import Path
import sys
import tempfile
import unittest
from unittest.mock import patch


SOURCE = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(SOURCE))

spec = importlib.util.spec_from_file_location("provision_target", SOURCE / "provision-target.py")
provision_target = importlib.util.module_from_spec(spec)
spec.loader.exec_module(provision_target)


class ProvisionTargetTest(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        self.write("etc/os-release", 'ID=fedora\nVERSION_ID="44"\n')
        self.write("etc/passwd", "root:x:0:0:root:/root:/bin/bash\n")
        self.write("etc/group", "root:x:0:\nwheel:x:10:\n")
        self.write("etc/shadow", "root:!::0:99999:7:::\n")
        self.write("etc/pam.d/sddm", "auth substack password-auth\nsession include postlogin\n")
        for binary in provision_target.REQUIRED_BINARIES:
            self.write(binary, "#!/bin/sh\nexit 0\n")
        self.write("usr/bin/ryoku", "#!/bin/sh\nexit 0\n")
        self.write("usr/share/ryoku/config/sample.conf", "sample config\n")
        self.write("usr/share/ryoku/wallpapers/default.png", "image\n")
        self.write("usr/share/ryoku/ryodecors/card.png", "decor\n")
        self.write("usr/share/ryoku/brand/logo.svg", "<svg></svg>\n")
        self.write("usr/share/ryoku/apps/mimeapps.list", "[Default Applications]\ntext/plain=ryoku-nvim.desktop\n")
        self.write("usr/share/ryoku/config/npm/npmrc", "prefix=~/.local\n")
        self.write("usr/share/ryoku/lockscreen/qylock/themes/clockwork/orbital/Main.qml", "import QtQuick\n")
        self.write("usr/share/ryoku/lockscreen/qylock/quickshell-lockscreen/lock.sh", "#!/bin/sh\n")

    def write(self, relative, content):
        path = self.root / relative
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(content)

    def test_validate_target_requires_fedora_44_and_binaries(self):
        provision_target.validate_target(self.root)

        self.write("etc/os-release", 'ID=fedora\nVERSION_ID="43"\n')
        with self.assertRaisesRegex(ValueError, "Fedora 44"):
            provision_target.validate_target(self.root)

        self.write("etc/os-release", 'ID=arch\nVERSION_ID="rolling"\n')
        with self.assertRaisesRegex(ValueError, "Fedora 44"):
            provision_target.validate_target(self.root)

        self.write("etc/os-release", 'ID=fedora\nVERSION_ID="44"\n')
        (self.root / "usr/bin/fish").unlink()
        with self.assertRaisesRegex(ValueError, "Missing target dependency"):
            provision_target.validate_target(self.root)

    def test_validate_target_rejects_running_system(self):
        with self.assertRaisesRegex(ValueError, "running system"):
            provision_target.validate_target(Path("/"))

    def test_configure_accounts_and_sudo_precreates_ryoku_and_locks_root(self):
        self.write("etc/shadow", "root::0:0:99999:7:::\n")
        self.write("etc/sudoers.d/99-test-nopasswd", "%wheel ALL=(ALL) NOPASSWD: ALL\n")

        provision_target.configure_accounts_and_sudo(self.root)

        passwd = (self.root / "etc/passwd").read_text()
        self.assertIn("ryoku:x:1000:1000:Ryoku Desktop:/home/ryoku:/usr/bin/fish", passwd)

        group = (self.root / "etc/group").read_text()
        self.assertIn("wheel:x:10:ryoku", group)

        shadow = (self.root / "etc/shadow").read_text()
        self.assertIn("root:!:", shadow)
        self.assertIn("ryoku:!:", shadow)

        wheel_sudo = self.root / "etc/sudoers.d/10-ryoku-wheel"
        self.assertTrue(wheel_sudo.exists())
        self.assertEqual(wheel_sudo.read_text(), "%wheel ALL=(ALL:ALL) ALL\n")
        self.assertEqual(wheel_sudo.stat().st_mode & 0o777, 0o440)
        self.assertFalse((self.root / "etc/sudoers.d/99-test-nopasswd").exists())

    def test_configure_accounts_normalizes_existing_user(self):
        self.write("etc/passwd", "root:x:0:0:root:/root:/bin/bash\nryoku:x:1001:1001:Ryoku:/home/other:/bin/bash\n")
        self.write("etc/group", "root:x:0:\nwheel:x:10:\n")
        self.write("etc/shadow", "root:!::0:99999:7:::\nryoku:!::0:99999:7:::\n")

        provision_target.configure_accounts_and_sudo(self.root)

        passwd = (self.root / "etc/passwd").read_text()
        self.assertIn("ryoku:x:1001:1001:Ryoku:/home/ryoku:/usr/bin/fish", passwd)
        self.assertIn("wheel:x:10:ryoku", (self.root / "etc/group").read_text())

    def test_configure_session_and_greeter(self):
        provision_target.configure_session_and_greeter(self.root)

        wayland_conf = self.root / "etc/sddm.conf.d/10-ryoku-wayland.conf"
        self.assertTrue(wayland_conf.exists())
        self.assertIn("DisplayServer=wayland", wayland_conf.read_text())

        sddm_conf = self.root / "etc/sddm.conf.d/99-ryoku.conf"
        self.assertTrue(sddm_conf.exists())
        content = sddm_conf.read_text()
        self.assertIn("Current=ryoku", content)
        self.assertIn("Session=niri.desktop", content)

        cursor_theme = self.root / "usr/share/icons/default/index.theme"
        self.assertTrue(cursor_theme.exists())
        self.assertIn("Inherits=Bibata-Modern-Ice", cursor_theme.read_text())

        dm_link = self.root / "etc/systemd/system/display-manager.service"
        self.assertTrue(dm_link.is_symlink())
        self.assertEqual(dm_link.readlink(), Path("/usr/lib/systemd/system/sddm.service"))

        def_target = self.root / "etc/systemd/system/default.target"
        self.assertTrue(def_target.is_symlink())
        self.assertEqual(def_target.readlink(), Path("/usr/lib/systemd/system/graphical.target"))

        pam_sddm = (self.root / "etc/pam.d/sddm").read_text()
        self.assertIn("pam_gnome_keyring.so", pam_sddm)

    def test_enable_base_services(self):
        provision_target.enable_base_services(self.root)

        wants = self.root / "etc/systemd/system/multi-user.target.wants"
        self.assertTrue((wants / "NetworkManager.service").is_symlink())
        self.assertTrue((wants / "firewalld.service").is_symlink())

        bt_wants = self.root / "etc/systemd/system/bluetooth.target.wants"
        self.assertTrue((bt_wants / "bluetooth.service").is_symlink())

    def test_seed_lockscreen(self):
        provision_target.seed_lockscreen(self.root)

        user_home = self.root / "home/ryoku"
        lock_sh = user_home / ".local/share/quickshell-lockscreen/lock.sh"
        self.assertTrue(lock_sh.exists())
        self.assertTrue(lock_sh.stat().st_mode & 0o111)

        orbital_main = user_home / ".local/share/qylock/themes/clockwork/orbital/Main.qml"
        self.assertTrue(orbital_main.exists())

        themes_link = user_home / ".local/share/quickshell-lockscreen/themes_link"
        self.assertTrue(themes_link.is_symlink())

        theme_pref = user_home / ".config/qylock/theme"
        self.assertTrue(theme_pref.exists())
        self.assertEqual(theme_pref.read_text(), "clockwork/orbital\n")

    def test_seed_assets_and_integration(self):
        provision_target.seed_assets_and_integration(self.root)

        user_home = self.root / "home/ryoku"
        self.assertTrue((user_home / "Pictures/Wallpapers/default.png").exists())
        self.assertTrue((user_home / "Pictures/ryodecors/card.png").exists())
        self.assertTrue((user_home / ".local/share/ryoku/assets/brand/logo.svg").exists())
        self.assertTrue((user_home / ".npmrc").exists())

        mime_apps = self.root / "usr/share/applications/mimeapps.list"
        self.assertTrue(mime_apps.exists())

    def test_materialize_config_runs_command_or_copies(self):
        calls = []

        def mock_runner(cmd):
            calls.append(cmd)

        provision_target.materialize_config(self.root, runner=mock_runner)
        self.assertEqual(len(calls), 1)
        self.assertIn("materialize", calls[0])

    def test_materialize_config_applies_niri_when_present(self):
        calls = []
        self.write("usr/bin/ryoku-wm-niri", "#!/bin/sh\n")

        def mock_runner(cmd):
            calls.append(cmd)

        provision_target.materialize_config(self.root, runner=mock_runner)
        self.assertEqual(len(calls), 2)
        self.assertIn("materialize", calls[0])
        self.assertIn("ryoku-wm-niri", calls[1][11])
        self.assertIn("apply", calls[1][12])


    def test_arm_firstboot_invokes_prepare_script(self):
        calls = []

        def mock_runner(cmd):
            calls.append(cmd)

        provision_target.arm_firstboot(self.root, runner=mock_runner)
        self.assertEqual(len(calls), 1)
        self.assertIn("prepare-firstboot.py", calls[0][1])

    def test_relabel_selinux(self):
        self.write("etc/selinux/targeted/contexts/files/file_contexts", "/.* system_u:object_r:default_t:s0\n")
        calls = []

        def mock_runner(cmd):
            calls.append(cmd)

        provision_target.relabel_selinux(self.root, runner=mock_runner)
        self.assertEqual(len(calls), 1)
        self.assertEqual(calls[0][0], "setfiles")

    def test_anaconda_preserves_accounts_and_keyboard(self):
        self.write("etc/passwd", "root:x:0:0:root:/root:/bin/bash\nalice:x:1001:1001:Alice:/home/alice:/bin/bash\n")
        self.write("etc/group", "root:x:0:\nwheel:x:10:alice\nalice:x:1001:\n")
        self.write("etc/shadow", "root:!::0:99999:7:::\nalice:$6$testhash::0:99999:7:::\n")
        self.write("etc/vconsole.conf", "KEYMAP=de\n")
        self.write("etc/X11/xorg.conf.d/00-keyboard.conf", 'Option "XkbLayout" "de"\n')
        self.write("etc/locale.conf", "LANG=de_AT.UTF-8\n")
        preserved = {path: path.read_bytes() for path in (self.root / "etc").rglob("*") if path.is_file()}
        calls = []
        with patch.object(provision_target, "arm_firstboot") as arm:
            provision_target.provision(self.root, runner=calls.append, anaconda=True)
            arm.assert_not_called()
        for path, content in preserved.items():
            if path.name != "sddm":
                self.assertEqual(path.read_bytes(), content, str(path))
        self.assertFalse((self.root / "home/ryoku").exists())
        self.assertTrue((self.root / "home/alice/Pictures/Wallpapers/default.png").is_file())
        self.assertIn("alice", calls[0])
        self.assertIn("HOME=/home/alice", calls[0])
        self.assertFalse((self.root / "var/lib/ryoku-firstboot/armed").exists())

    def test_anaconda_requires_a_login_account(self):
        with self.assertRaisesRegex(ValueError, "Create a login account"):
            provision_target.provision(self.root, anaconda=True)
        self.assertFalse((self.root / "home/ryoku").exists())

    def test_full_provision_flow(self):
        calls = []

        def mock_runner(cmd):
            calls.append(cmd)

        with patch.object(provision_target, "arm_firstboot") as mock_arm:
            provision_target.provision(self.root, runner=mock_runner)
            mock_arm.assert_called_once_with(self.root, runner=mock_runner)

        user_home = self.root / "home/ryoku"
        self.assertTrue((user_home / "Pictures/Wallpapers").is_dir())
        self.assertTrue((self.root / "etc/sddm.conf.d/99-ryoku.conf").exists())
        self.assertTrue((self.root / "etc/sudoers.d/10-ryoku-wheel").exists())
        self.assertTrue((self.root / "etc/systemd/system/display-manager.service").is_symlink())


if __name__ == "__main__":
    unittest.main()
