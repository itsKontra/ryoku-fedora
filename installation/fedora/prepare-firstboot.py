#!/usr/bin/python3
"""Arm console setup in a fresh, mounted Fedora installation target."""

import argparse
import os
from pathlib import Path
import shutil
import sys

from firstboot import SETTINGS, STATE, password_set


def prepare(root):
    root = root.resolve(strict=True)
    if root == Path("/") or (root / "run/systemd/system").exists():
        raise ValueError("Expected an offline installation target, not a running system")
    release = (root / "etc/os-release").read_text()
    values = dict(line.split("=", 1) for line in release.splitlines() if "=" in line)
    if values.get("ID", "").strip('"') != "fedora" or values.get("VERSION_ID", "").strip('"') != "44":
        raise ValueError("Expected a Fedora 44 installation target")
    state = root / STATE
    if (state / "armed").exists():
        return
    accounts = dict((p[0], p) for p in (line.split(":") for line in (root / "etc/passwd").read_text().splitlines()))
    user = accounts.get("ryoku")
    if not user or user[2] == "0" or user[5:] != ["/home/ryoku", "/usr/bin/fish"]:
        raise ValueError("Pre-create ryoku with /home/ryoku and /usr/bin/fish")
    groups = [line.split(":") for line in (root / "etc/group").read_text().splitlines()]
    if not any(g[0] == "wheel" and ("ryoku" in g[3].split(",") or g[2] == user[3]) for g in groups):
        raise ValueError("ryoku must belong to wheel")
    for account in ("root", "ryoku"):
        if password_set(root, account):
            raise ValueError(f"Refusing to reset an existing {account} password")
        entry = next(line.split(":") for line in (root / "etc/shadow").read_text().splitlines() if line.startswith(account + ":"))
        if not entry[1]:
            raise ValueError(f"The {account} account must be locked, not passwordless")
        if entry[1] not in ("!", "!!", "*", "!*"):
            raise ValueError(f"Refusing to reset an existing locked {account} password")
    for relative in ("usr/bin/python3", "usr/bin/systemd-firstboot", "usr/bin/passwd", "usr/bin/fish"):
        if not (root / relative).is_file():
            raise ValueError(f"Missing target dependency: {relative}")
    source = Path(__file__).resolve().parent
    for name, destination, mode in (
        ("firstboot.py", "usr/libexec/ryoku-firstboot", 0o755),
        ("ryoku-firstboot.service", "etc/systemd/system/ryoku-firstboot.service", 0o644),
        ("firstboot-gate.conf", "etc/systemd/system/sddm.service.d/10-firstboot.conf", 0o644),
    ):
        target = root / destination
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copyfile(source / name, target)
        target.chmod(mode)
    units = root / "etc/systemd/system"
    wants = units / "multi-user.target.wants"
    wants.mkdir(exist_ok=True)
    link = wants / "ryoku-firstboot.service"
    link.unlink(missing_ok=True)
    link.symlink_to("../ryoku-firstboot.service")
    # The stock unit runs earlier and can consume seeded state or omit hostname.
    stock = units / "systemd-firstboot.service"
    stock.unlink(missing_ok=True)
    stock.symlink_to("/dev/null")
    state.mkdir(parents=True, exist_ok=True, mode=0o700)
    for _, relative in SETTINGS:
        (root / relative).unlink(missing_ok=True)
    # Anaconda will install per-machine state; never reuse the compose identity.
    machine_id = root / "etc/machine-id"
    machine_id.unlink(missing_ok=True)
    machine_id.write_text("uninitialized\n")
    dbus_id = root / "var/lib/dbus/machine-id"
    dbus_id.parent.mkdir(parents=True, exist_ok=True)
    dbus_id.unlink(missing_ok=True)
    dbus_id.symlink_to("/etc/machine-id")
    os.sync()
    (state / "armed").write_text("1\n")
    os.sync()


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("root", type=Path, help="mounted target root after package installation")
    args = parser.parse_args()
    try:
        if os.geteuid() != 0:
            raise ValueError("Preparing an installation target requires root")
        prepare(args.root)
    except (OSError, ValueError) as error:
        sys.exit(str(error))
