#!/usr/bin/python3
"""Complete the ISO target's console setup before its display manager starts."""

import fcntl
import os
from pathlib import Path
import subprocess
import sys


SETTINGS = (
    ("locale", "etc/locale.conf"),
    ("keymap", "etc/vconsole.conf"),
    ("timezone", "etc/localtime"),
    ("hostname", "etc/hostname"),
)
STATE = "var/lib/ryoku-firstboot"


def password_set(root, user):
    for line in (root / "etc/shadow").read_text().splitlines():
        fields = line.split(":")
        if fields[0] == user:
            return bool(fields[1]) and fields[1][0] not in "!*"
    raise ValueError(f"Missing account: {user}")


def setting_set(root, name, relative):
    path = root / relative
    if name == "timezone":
        return path.is_symlink() and bool(os.readlink(path))
    if not path.is_file():
        return False
    content = path.read_text().strip()
    if name == "locale":
        return any(line.startswith("LANG=") and line[5:].strip('"\'') for line in content.splitlines())
    if name == "keymap":
        return any(line.startswith("KEYMAP=") and line[7:].strip('"\'') for line in content.splitlines())
    return bool(content)


def mark_complete(state):
    # Persist the chosen settings and credentials before allowing graphical login.
    os.sync()
    temporary = state / "complete.tmp"
    with temporary.open("w") as marker:
        marker.write("1\n")
        marker.flush()
        os.fsync(marker.fileno())
    temporary.replace(state / "complete")
    fd = os.open(state, os.O_RDONLY | os.O_DIRECTORY)
    try:
        os.fsync(fd)
    finally:
        os.close(fd)


def setup(root=Path("/"), run=subprocess.run):
    state = root / STATE
    if not (state / "armed").is_file():
        raise ValueError("First-boot setup was not armed by the installer")
    with (state / "lock").open("w") as lock:
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        if (state / "complete").is_file():
            return
        env = os.environ.copy()
        # Imported systemd credentials must not silently answer interactive setup.
        env.pop("CREDENTIALS_DIRECTORY", None)
        env.pop("ENCRYPTED_CREDENTIALS_DIRECTORY", None)
        env["SYSTEMD_PAGER"] = "cat"
        firstboot = ["systemd-firstboot", f"--root={root}", "--welcome=false"]
        print("Welcome to Ryoku. Complete these settings to open the login screen.", flush=True)
        for name, relative in SETTINGS:
            while not setting_set(root, name, relative):
                # --force also handles an empty file left by an interrupted write.
                run([*firstboot, "--force", f"--prompt-{name}"], check=True, env=env)
                os.sync()
                if not setting_set(root, name, relative):
                    print(f"A {name} is required; please choose a value.", flush=True)
        if root == Path("/") and Path("/run/systemd/system").is_dir():
            run(["systemctl", "daemon-reload"], check=True, env=env)
            # Passwords must be entered with the layout that the next login uses.
            run(["systemctl", "restart", "systemd-vconsole-setup.service"], check=True, env=env)
        while not password_set(root, "root"):
            run([*firstboot, "--force", "--prompt-root-password"], check=True, env=env)
            os.sync()
            if not password_set(root, "root"):
                print("A root password is required.", flush=True)
        while not password_set(root, "ryoku"):
            print("Choose the password for your ryoku login account.", flush=True)
            run(["passwd", "--root", str(root), "ryoku"], check=True, env=env)
            os.sync()
        mark_complete(state)


if __name__ == "__main__":
    try:
        if os.geteuid() != 0:
            raise ValueError("First-boot setup requires root")
        setup()
    except (OSError, ValueError, subprocess.CalledProcessError, KeyboardInterrupt) as error:
        print(f"Setup incomplete: {error}. Reboot to resume.", file=sys.stderr)
        sys.exit(1)
