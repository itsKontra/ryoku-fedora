import hashlib
import importlib.util
import json
from pathlib import Path
import shutil
import subprocess
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
        for asset in (
            "share/icons/Bibata-Modern-Ice/cursors/left_ptr",
            "share/fonts/SpaceGrotesk/regular.otf",
            "share/fonts/MaterialSymbolsRounded.ttf",
            "share/fonts/JetBrainsMonoNerdFont/regular.ttf",
            "share/fonts/SpaceMonoNerdFont/regular.ttf",
            "bin/matugen",
        ):
            self.write("usr/" + asset, "installed asset\n")
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
        self.assertIn("Session=hyprland.desktop", content)

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
        self.assertTrue((wants / "power-profiles-daemon.service").is_symlink())
        self.assertTrue((wants / "ryoku-boot-guard.service").is_symlink())

        bt_wants = self.root / "etc/systemd/system/bluetooth.target.wants"
        self.assertTrue((bt_wants / "bluetooth.service").is_symlink())

    def test_initialize_boot_guard(self):
        provision_target.initialize_boot_guard(self.root)

        boot_var = self.root / "var/lib/ryoku/boot"
        self.assertTrue(boot_var.is_dir())
        self.assertEqual(boot_var.stat().st_mode & 0o1777, 0o1777)

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

    def test_materialize_config_runs_as_target_user(self):
        provision_target.configure_accounts_and_sudo(self.root)
        calls = []

        def mock_runner(cmd):
            calls.append(cmd)

        provision_target.materialize_config(self.root, runner=mock_runner)
        self.assertEqual(len(calls), 2)
        self.assertIn("materialize", calls[0])

    def test_materialize_config_applies_hyprland(self):
        provision_target.configure_accounts_and_sudo(self.root)
        calls = []
        self.write("usr/bin/ryoku-wm-hyprland", "#!/bin/sh\n")

        def mock_runner(cmd):
            calls.append(cmd)

        provision_target.materialize_config(self.root, runner=mock_runner)
        self.assertEqual(len(calls), 2)
        self.assertIn("materialize", calls[0])
        self.assertIn("ryoku-wm-hyprland", calls[1][11])
        self.assertIn("apply", calls[1][12])


    def test_materialization_failures_abort_before_firstboot(self):
        for failed_command in ("materialize", "apply"):
            with self.subTest(command=failed_command):
                sudoers = self.root / "etc/sudoers.d/10-ryoku-wheel"
                if sudoers.exists():
                    sudoers.chmod(0o600)

                def fail(cmd):
                    if failed_command in cmd:
                        raise subprocess.CalledProcessError(1, cmd)

                with patch.object(provision_target, "arm_firstboot") as arm:
                    with self.assertRaises(subprocess.CalledProcessError):
                        provision_target.provision(self.root, runner=fail)
                    arm.assert_not_called()
                self.assertFalse((self.root / "home/ryoku/.config/hypr/settings.lua").exists())

    def test_extras_load_extensionless_helper_without_changing_command_discovery(self):
        font = self.root / "usr/share/fonts/MaterialSymbolsRounded.ttf"
        font.unlink()
        repo = self.root / "repo"
        script = repo / "ryoku/shell/scripts/ryoku-install-extra"
        script.parent.mkdir(parents=True)
        script.write_text(
            "import shutil\n"
            "def install(name, root):\n"
            "    assert name == 'material-symbols'\n"
            "    (root / 'share/fonts/MaterialSymbolsRounded.ttf').write_bytes(b'font')\n"
        )
        which = shutil.which
        provision_target.seed_desktop_extras(self.root, repo)
        self.assertEqual(font.read_bytes(), b"font")
        self.assertIs(shutil.which, which)

    def test_real_extras_helper_installs_verified_cached_font(self):
        font = self.root / "usr/share/fonts/MaterialSymbolsRounded.ttf"
        font.unlink()
        data = b"\x00\x01\x00\x00test font"
        digest = hashlib.sha256(data).hexdigest()
        cache = self.root / "cache"
        cache.mkdir()
        (cache / digest).write_bytes(data)
        repo = self.root / "repo"
        helper = repo / "ryoku/shell/scripts/ryoku-install-extra"
        helper.parent.mkdir(parents=True)
        source = SOURCE.parents[1] / "ryoku/shell/scripts/ryoku-install-extra"
        helper.write_text(source.read_text() + (
            f"\nRELEASES['material-symbols'] = ('test', 'https://example.invalid/font', "
            f"'{digest}', 'share/fonts/MaterialSymbolsRounded.ttf')\n"
        ))
        with patch.dict("os.environ", {"RYOKU_EXTRA_CACHE": str(cache)}):
            provision_target.seed_desktop_extras(self.root, repo)
        self.assertEqual(font.read_bytes(), data)
        receipt = self.root / "usr/state/ryoku/extras/material-symbols.json"
        self.assertEqual(json.loads(receipt.read_text())["sha256"], digest)

    def test_real_extras_helper_reports_download_failure(self):
        (self.root / "usr/share/fonts/MaterialSymbolsRounded.ttf").unlink()
        with patch("urllib.request.urlopen", side_effect=OSError("download unavailable")):
            with self.assertRaisesRegex(ValueError, "material-symbols: download unavailable"):
                provision_target.seed_desktop_extras(self.root)

    def test_extras_require_helper_and_installed_asset(self):
        (self.root / "usr/share/fonts/MaterialSymbolsRounded.ttf").unlink()
        repo = self.root / "repo"
        repo.mkdir()
        with self.assertRaisesRegex(ValueError, "Missing desktop extras helper"):
            provision_target.seed_desktop_extras(self.root, repo)
        helper = repo / "ryoku/shell/scripts/ryoku-install-extra"
        helper.parent.mkdir(parents=True)
        helper.write_text("def install(name, root): pass\n")
        with self.assertRaisesRegex(ValueError, "Missing required desktop extra.*material-symbols"):
            provision_target.seed_desktop_extras(self.root, repo)

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
        desktop_cfg = json.loads((self.root / "home/alice/.config/ryoku/desktop.json").read_text())
        self.assertEqual(desktop_cfg["desktop"]["input"]["kbLayout"], "de")
        shell_cfg = json.loads((self.root / "home/alice/.config/ryoku/shell.json").read_text())
        self.assertEqual(shell_cfg["language"], "de")
        self.assertFalse((self.root / "var/lib/ryoku-firstboot/armed").exists())

    def test_anaconda_requires_a_login_account(self):
        with self.assertRaisesRegex(ValueError, "Create a login account"):
            provision_target.provision(self.root, anaconda=True)
        self.assertFalse((self.root / "home/ryoku").exists())

    def test_anaconda_requires_password_protected_administrator(self):
        self.write("etc/passwd", "root:x:0:0:root:/root:/bin/bash\nalice:x:1001:1001:Alice:/home/alice:/bin/bash\n")
        for password, member in (("!", "alice"), ("!$6$hash", "alice"), ("*", "alice"), ("", "alice"), ("$6$hash", "")):
            with self.subTest(password=password, member=member):
                self.write("etc/shadow", f"root:!::0:99999:7:::\nalice:{password}::0:99999:7:::\n")
                self.write("etc/group", f"wheel:x:10:{member}\n")
                with self.assertRaisesRegex(ValueError, "administrator"):
                    provision_target.anaconda_accounts(self.root)
        self.write("etc/group", "wheel:x:10:alice\n")
        self.assertEqual(provision_target.anaconda_accounts(self.root), [("alice", "/home/alice")])

    def test_normalize_dnf_repository_migrates_legacy_name(self):
        self.write("etc/yum.repos.d/ryoku.repo", "[ryoku]\nbaseurl=https://example.test/repo\ngpgcheck=1\n")
        provision_target.normalize_dnf_repositories(self.root)
        canonical = self.root / "etc/yum.repos.d/RyokuCOPR.repo"
        self.assertIn("[RyokuCOPR]", canonical.read_text())
        self.assertFalse((self.root / "etc/yum.repos.d/ryoku.repo").exists())
        provision_target.normalize_dnf_repositories(self.root)
        self.assertIn("gpgcheck=1", canonical.read_text())

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
        self.assertTrue((self.root / "usr/bin/ryostore-install").is_file())
        self.assertTrue((self.root / "usr/share/polkit-1/rules.d/52-ryoku-timedate.rules").is_file())
        self.assertTrue((self.root / "usr/lib/udev/rules.d/90-ryoku-gpu.rules").is_file())
        self.assertTrue((self.root / "etc/modules-load.d/ryoku-i2c.conf").is_file())
        self.assertTrue((self.root / "usr/lib/modprobe.d/99-ryoku-controller.conf").is_file())
        self.assertTrue((self.root / "etc/systemd/logind.conf.d/10-ryoku-lid.conf").is_file())


    def test_normalize_dnf_repositories_populates_copr_gpgkeys(self):
        copr_file = "etc/yum.repos.d/RyotunesCOPR.repo"
        content = (
            "[RyotunesCOPR]\n"
            "name = RyotunesCOPR\n"
            "enabled = 1\n"
            "baseurl = https://download.copr.fedorainfracloud.org/results/itskontra/ryotunes/fedora-44-x86_64/\n"
            "cost = 20\n"
        )
        self.write(copr_file, content)
        provision_target.normalize_dnf_repositories(self.root)
        updated = (self.root / copr_file).read_text()
        self.assertIn("gpgcheck = 1", updated)
        self.assertIn("gpgkey = https://download.copr.fedorainfracloud.org/results/itskontra/ryotunes/pubkey.gpg", updated)

    def test_seed_assets_falls_back_to_repo_dir_when_target_lacks_wallpapers(self):
        # Remove target wallpapers so target lacks usr/share/ryoku/wallpapers
        shutil.rmtree(self.root / "usr/share/ryoku/wallpapers")
        with tempfile.TemporaryDirectory() as temp_repo:
            repo_path = Path(temp_repo)
            wall_dir = repo_path / "ryoku/assets/wallpapers"
            wall_dir.mkdir(parents=True, exist_ok=True)
            (wall_dir / "fallback.png").write_text("fallback wallpaper\n")

            user_home = self.root / "home/ryoku"
            provision_target.seed_assets_and_integration(self.root, repo_dir=repo_path, home="/home/ryoku")
            self.assertTrue((user_home / "Pictures/Wallpapers/fallback.png").is_file())

    def test_install_system_extras(self):
        provision_target.install_system_extras(self.root)
        bin_dir = self.root / "usr/bin"
        for name in (
            "ryostore-install",
            "ryoku-pkg-add",
            "ryoku-pkg-remove",
            "ryoku-pkg-aur-add",
            "ryoku-pkg-multilib",
            "ryoku-cmd-present",
        ):
            target_bin = bin_dir / name
            self.assertTrue(target_bin.is_file(), f"Missing extras binary: {name}")
            self.assertEqual(target_bin.stat().st_mode & 0o777, 0o755)

    def test_install_policy_rules(self):
        provision_target.install_policy_rules(self.root)
        polkit_dir = self.root / "usr/share/polkit-1/rules.d"
        for rule in (
            "52-ryoku-timedate.rules",
            "46-ryoku-docker.rules",
            "47-ryoku-power.rules",
            "48-ryoku-wifi-regdom.rules",
            "49-ryoku-wifi-powersave.rules",
            "50-ryoku-dns.rules",
            "51-ryoku-wifi-backend.rules",
            "53-ryoku-game-tune.rules",
            "54-ryoku-bluetooth-a2dp.rules",
            "55-ryoku-network-kill.rules",
        ):
            target_rule = polkit_dir / rule
            self.assertTrue(target_rule.is_file(), f"Missing polkit rule: {rule}")
            self.assertEqual(target_rule.stat().st_mode & 0o777, 0o644)

    def test_install_hardware_support(self):
        self.write("etc/bluetooth/main.conf", "[General]\n#AutoEnable=true\n")
        provision_target.install_hardware_support(self.root)

        # Helpers
        bin_dir = self.root / "usr/bin"
        for name in ("ryoku-gpu", "ryoku-power", "ryoku-wifi-regdom", "ryoku-docker"):
            target_bin = bin_dir / name
            self.assertTrue(target_bin.is_file(), f"Missing helper: {name}")
            self.assertEqual(target_bin.stat().st_mode & 0o777, 0o755)

        # Udev
        udev_dir = self.root / "usr/lib/udev/rules.d"
        for rule in ("90-ryoku-gpu.rules", "90-ryoku-backlight.rules", "60-ryoku-i2c.rules", "70-ryoku-maono.rules"):
            target_rule = udev_dir / rule
            self.assertTrue(target_rule.is_file(), f"Missing udev rule: {rule}")
            self.assertEqual(target_rule.stat().st_mode & 0o777, 0o644)

        # Module loading
        self.assertTrue((self.root / "etc/modules-load.d/ryoku-i2c.conf").is_file())
        self.assertTrue((self.root / "etc/modules-load.d/99-ryoku-uinput.conf").is_file())

        # Modprobe
        self.assertTrue((self.root / "usr/lib/modprobe.d/99-ryoku-audio-powersave.conf").is_file())
        self.assertTrue((self.root / "usr/lib/modprobe.d/99-ryoku-controller.conf").is_file())
        self.assertTrue((self.root / "usr/lib/modprobe.d/99-ryoku-bt-autosuspend.conf").is_file())

        # Logind
        lid_conf = self.root / "etc/systemd/logind.conf.d/10-ryoku-lid.conf"
        self.assertTrue(lid_conf.is_file())
        self.assertEqual(lid_conf.stat().st_mode & 0o777, 0o644)

        # Services
        self.assertTrue((self.root / "usr/lib/systemd/system/ryoku-wifi-regdom.service").is_file())
        self.assertTrue((self.root / "usr/lib/systemd/user/ryoku-bluetooth-reset.service").is_file())
        self.assertTrue((self.root / "etc/systemd/system/multi-user.target.wants/ryoku-wifi-regdom.service").is_symlink())
        self.assertTrue((self.root / "etc/systemd/user/default.target.wants/ryoku-bluetooth-reset.service").is_symlink())

        # BlueZ tuning applied
        bt_content = (self.root / "etc/bluetooth/main.conf").read_text()
        self.assertIn("AutoEnable = true", bt_content)

    def test_run_hardware_drivers_stages_vendor_scripts(self):
        provision_target.run_hardware_drivers(self.root)
        drivers_dir = self.root / "usr/share/ryoku/hardware/drivers"
        for script in ("amd.sh", "intel.sh", "vulkan.sh", "common.sh"):
            target_script = drivers_dir / script
            self.assertTrue(target_script.is_file(), f"Missing driver script: {script}")
            self.assertEqual(target_script.stat().st_mode & 0o777, 0o755)

    @patch("shutil.which", return_value="/usr/sbin/chroot")
    def test_run_hardware_drivers_installs_signed_nvidia_driver(self, _which):
        (self.root / "usr/bin").mkdir(parents=True, exist_ok=True)
        (self.root / "usr/bin/bash").write_text("")
        calls = []
        provision_target.run_hardware_drivers(self.root, runner=calls.append)
        self.assertNotIn(["chroot", str(self.root), "/usr/bin/ryoku-nvidia", "install"], calls)

        (self.root / "usr/bin/ryoku-nvidia").write_text("")
        calls.clear()
        provision_target.run_hardware_drivers(self.root, runner=calls.append)
        self.assertEqual(calls[-1], ["chroot", str(self.root), "/usr/bin/ryoku-nvidia", "install"])


if __name__ == "__main__":
    unittest.main()

