#!/usr/bin/env python3
"""Pre-installation validator for Ryoku Desktop on Fedora.

Validates that at least one user account with administrator privileges
(wheel group) and a non-empty password has been configured in Anaconda
before package installation begins.

This script runs during Kickstart %pre-install (--nochroot --erroronfail)
to halt installation early if the requirement is not met, preventing
wasted package download/installation time before a %post failure.
"""

import argparse
import ast
import os
import re
import sys
from pathlib import Path


def check_dbus():
    """Query Anaconda DBus Users module if available."""
    try:
        from pyanaconda.modules.common.constants.services import USERS
        from pyanaconda.modules.common.structures.user import UserData

        users_proxy = USERS.get_proxy()
        raw_users = getattr(users_proxy, "Users", None)
        if raw_users is None:
            return None, "Anaconda Users DBus property not accessible"

        user_list = UserData.from_structure_list(raw_users)
        if not user_list:
            return False, "No user accounts have been created in Anaconda User Creation."

        for u in user_list:
            has_wheel = "wheel" in getattr(u, "groups", [])
            has_password = bool(getattr(u, "password", ""))
            is_locked = bool(getattr(u, "lock", False))

            if has_wheel and has_password and not is_locked:
                return True, f"Configured administrator '{u.name}' in wheel with password."
            elif has_wheel and not has_password:
                return False, f"User '{u.name}' is an administrator (wheel) but has no password set. A password is required."
            elif not has_wheel and has_password:
                return False, f"User '{u.name}' has a password but lacks administrator privileges ('Make this user administrator' is required)."

        return False, "No administrator account in wheel with a non-empty password was found."
    except Exception as exc:
        return None, f"DBus check unavailable: {exc}"


def check_logs(log_paths=None):
    """Fallback check parsing Anaconda installer logs for UserData entries."""
    if log_paths is None:
        log_paths = [Path("/tmp/syslog"), Path("/tmp/anaconda.log")]

    for path in log_paths:
        p = Path(path)
        if not p.is_file():
            continue
        try:
            content = p.read_text(encoding="utf-8", errors="replace")
        except OSError:
            continue

        # Find all UserData records logged by Anaconda
        matches = re.findall(r"UserData\([^)]+\)", content)
        if not matches:
            continue

        last_match = matches[-1]

        # Extract fields from UserData representation
        has_wheel = ("'wheel'" in last_match) or ('"wheel"' in last_match)
        password_set = "password_set=True" in last_match
        password_unset = "password_set=False" in last_match
        is_locked = "lock=True" in last_match

        name_match = re.search(r"name=['\"]([^'\"]+)['\"]", last_match)
        user_name = name_match.group(1) if name_match else "user"

        if has_wheel and password_set and not is_locked:
            return True, f"Configured administrator '{user_name}' in wheel with password (from {p.name})."
        elif has_wheel and password_unset:
            return False, f"User '{user_name}' was created as administrator (wheel) but without a password. A password is required."
        elif not has_wheel and password_set:
            return False, f"User '{user_name}' has a password but is not an administrator (wheel membership required)."

    return None, "No user records found in installer logs"


def check_kickstart(ks_paths=None):
    """Fallback check parsing output or input kickstart for user commands."""
    if ks_paths is None:
        ks_paths = [Path("/tmp/anaconda-ks.cfg"), Path("/run/install/ks.cfg")]

    for path in ks_paths:
        p = Path(path)
        if not p.is_file():
            continue
        try:
            lines = p.read_text(encoding="utf-8", errors="replace").splitlines()
        except OSError:
            continue

        for line in lines:
            stripped = line.strip()
            if stripped.startswith("user "):
                has_wheel = ("--groups=" in stripped and "wheel" in stripped) or ("wheel" in stripped)
                has_password = "--password=" in stripped
                is_locked = "--lock" in stripped

                name_match = re.search(r"--name=([^\s]+)", stripped)
                name = name_match.group(1) if name_match else "user"

                if has_wheel and has_password and not is_locked:
                    return True, f"Configured administrator '{name}' in kickstart configuration."
                elif has_wheel and not has_password:
                    return False, f"User '{name}' in kickstart has no password configured."

    return None, "No user definitions found in kickstart files"


def validate_admin(syslog_path=None, ks_path=None):
    """Validate administrator requirements across DBus, logs, and Kickstart."""
    # 1. DBus verification (primary runtime check)
    if not syslog_path and not ks_path:
        dbus_ok, dbus_msg = check_dbus()
        if dbus_ok is True:
            return True, dbus_msg
        elif dbus_ok is False:
            return False, dbus_msg

    # 2. Installer log verification (handles interactive Anaconda GUI)
    log_targets = [Path(syslog_path)] if syslog_path else None
    log_ok, log_msg = check_logs(log_targets)
    if log_ok is True:
        return True, log_msg
    elif log_ok is False:
        return False, log_msg

    # 3. Kickstart file verification (handles unattended installs)
    ks_targets = [Path(ks_path)] if ks_path else None
    ks_ok, ks_msg = check_kickstart(ks_targets)
    if ks_ok is True:
        return True, ks_msg
    elif ks_ok is False:
        return False, ks_msg

    return False, "No administrator account with a password was configured before installation."


def main():
    parser = argparse.ArgumentParser(description="Validate Ryoku administrator account requirement.")
    parser.add_argument("--syslog", help="Path to syslog file for verification (testing).")
    parser.add_argument("--ks", help="Path to kickstart file for verification (testing).")
    args = parser.parse_args()

    ok, message = validate_admin(syslog_path=args.syslog, ks_path=args.ks)

    if ok:
        print(f":: Pre-installation check passed: {message}")
        sys.exit(0)
    else:
        sys.stderr.write(f"""
================================================================================
RYOKU INSTALLATION ERROR: Administrator Account Required
================================================================================
Validation failed: {message}

Ryoku requires a dedicated user account with administrator privileges
(wheel group) and a non-empty password before installation can begin.

Please return to the Anaconda installation hub:
  1. Click "User Creation"
  2. Enter your Full Name and Username
  3. Ensure "Make this user administrator" is checked
  4. Ensure "Require a password to use this account" is enabled
  5. Enter and confirm a secure password
  6. Click "Done" and verify the warning disappears before beginning installation
================================================================================
""")
        sys.exit(1)


if __name__ == "__main__":
    main()
