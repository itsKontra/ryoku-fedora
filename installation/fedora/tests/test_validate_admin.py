#!/usr/bin/env python3
"""Unit tests for the Ryoku administrator pre-installation validator."""

import importlib.util
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import MagicMock

REPO_ROOT = Path(__file__).resolve().parents[3]
VALIDATE_ADMIN_PATH = REPO_ROOT / "installation" / "fedora" / "validate-admin.py"

spec = importlib.util.spec_from_file_location("validate_admin", VALIDATE_ADMIN_PATH)
validate_admin = importlib.util.module_from_spec(spec)
spec.loader.exec_module(validate_admin)


class TestValidateAdmin(unittest.TestCase):
    """Test suite for administrator account validation logic."""

    def setUp(self):
        self.temp_dir = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp_dir.cleanup)
        self.temp_path = Path(self.temp_dir.name)

    def test_syslog_validation_succeeds_with_password_protected_wheel_user(self):
        syslog = self.temp_path / "syslog"
        syslog.write_text(
            "07:49:38,950 WARNING org.fedoraproject.Anaconda.Modules.Users:DEBUG:anaconda.modules.users.users:"
            "A new user list has been set: [UserData(gecos='Matthias', gid=0, gid_mode='ID_MODE_USE_DEFAULT', "
            "groups=['wheel'], homedir='', is_crypted=True, lock=False, name='matthias', password_set=True, "
            "shell='', uid=0, uid_mode='ID_MODE_USE_DEFAULT')]\n",
            encoding="utf-8",
        )
        ok, msg = validate_admin.validate_admin(syslog_path=str(syslog))
        self.assertTrue(ok)
        self.assertIn("matthias", msg)
        self.assertIn("wheel", msg)

    def test_syslog_validation_fails_when_user_has_no_password(self):
        # Reproduces exact VM 102 diagnosis log
        syslog = self.temp_path / "syslog"
        syslog.write_text(
            "07:49:38,950 WARNING org.fedoraproject.Anaconda.Modules.Users:DEBUG:anaconda.modules.users.users:"
            "A new user list has been set: [UserData(gecos='Matthias', gid=0, gid_mode='ID_MODE_USE_DEFAULT', "
            "groups=['wheel'], homedir='', is_crypted=False, lock=False, name='matthias', password_set=False, "
            "shell='', uid=0, uid_mode='ID_MODE_USE_DEFAULT')]\n",
            encoding="utf-8",
        )
        ok, msg = validate_admin.validate_admin(syslog_path=str(syslog))
        self.assertFalse(ok)
        self.assertIn("without a password", msg)
        self.assertIn("matthias", msg)

    def test_syslog_validation_fails_when_user_lacks_wheel(self):
        syslog = self.temp_path / "syslog"
        syslog.write_text(
            "07:49:38,950 WARNING org.fedoraproject.Anaconda.Modules.Users:DEBUG:anaconda.modules.users.users:"
            "A new user list has been set: [UserData(gecos='Regular User', gid=0, gid_mode='ID_MODE_USE_DEFAULT', "
            "groups=[], homedir='', is_crypted=True, lock=False, name='regular', password_set=True, "
            "shell='', uid=0, uid_mode='ID_MODE_USE_DEFAULT')]\n",
            encoding="utf-8",
        )
        ok, msg = validate_admin.validate_admin(syslog_path=str(syslog))
        self.assertFalse(ok)
        self.assertIn("not an administrator", msg)

    def test_kickstart_validation_succeeds_with_admin_user(self):
        ks = self.temp_path / "ks.cfg"
        ks.write_text("user --name=admin --groups=wheel --password=secret --plaintext\n", encoding="utf-8")
        ok, msg = validate_admin.validate_admin(ks_path=str(ks))
        self.assertTrue(ok)
        self.assertIn("admin", msg)

    def test_kickstart_validation_fails_when_user_has_no_password(self):
        ks = self.temp_path / "ks.cfg"
        ks.write_text("user --name=admin --groups=wheel\n", encoding="utf-8")
        ok, msg = validate_admin.validate_admin(ks_path=str(ks))
        self.assertFalse(ok)
        self.assertIn("no password configured", msg)

    def test_validation_fails_when_no_users_found(self):
        empty_log = self.temp_path / "syslog"
        empty_log.write_text("some random log message\n", encoding="utf-8")
        ok, msg = validate_admin.validate_admin(syslog_path=str(empty_log))
        self.assertFalse(ok)
        self.assertIn("No administrator account", msg)

    def test_dbus_check_success(self):
        class MockUser:
            def __init__(self, name, groups, password, lock):
                self.name = name
                self.groups = groups
                self.password = password
                self.lock = lock

        mock_users_service = MagicMock()
        mock_proxy = mock_users_service.get_proxy.return_value
        mock_proxy.Users = ["raw_user_struct"]
        mock_user_data = MagicMock()
        mock_user_data.from_structure_list.return_value = [
            MockUser("alice", ["wheel"], "hashed_pass", False)
        ]
        ok, msg = validate_admin.check_dbus(mock_users_service, mock_user_data)
        self.assertTrue(ok)
        self.assertIn("alice", msg)

    def test_dbus_check_rejects_empty_password(self):
        class MockUser:
            def __init__(self, name, groups, password, lock):
                self.name = name
                self.groups = groups
                self.password = password
                self.lock = lock

        mock_users_service = MagicMock()
        mock_proxy = mock_users_service.get_proxy.return_value
        mock_proxy.Users = ["raw_user_struct"]
        mock_user_data = MagicMock()
        mock_user_data.from_structure_list.return_value = [
            MockUser("matthias", ["wheel"], "", False)
        ]
        ok, msg = validate_admin.check_dbus(mock_users_service, mock_user_data)
        self.assertFalse(ok)
        self.assertIn("no password set", msg)

    def test_cli_execution_exits_0_on_valid_admin(self):
        syslog = self.temp_path / "syslog"
        syslog.write_text(
            "A new user list has been set: [UserData(name='matthias', groups=['wheel'], password_set=True, lock=False)]\n",
            encoding="utf-8",
        )
        res = subprocess.run(
            [sys.executable, str(VALIDATE_ADMIN_PATH), "--syslog", str(syslog)],
            capture_output=True,
            text=True,
        )
        self.assertEqual(res.returncode, 0)
        self.assertIn("Pre-installation check passed", res.stdout)

    def test_cli_execution_exits_1_on_missing_password(self):
        syslog = self.temp_path / "syslog"
        syslog.write_text(
            "A new user list has been set: [UserData(name='matthias', groups=['wheel'], password_set=False, lock=False)]\n",
            encoding="utf-8",
        )
        res = subprocess.run(
            [sys.executable, str(VALIDATE_ADMIN_PATH), "--syslog", str(syslog)],
            capture_output=True,
            text=True,
        )
        self.assertEqual(res.returncode, 1)
        self.assertIn("Administrator Account Required", res.stderr)
        self.assertIn("without a password", res.stderr)


if __name__ == "__main__":
    unittest.main()
