#!/usr/bin/python3
"""Exercise real Fedora prompts; run only in a disposable Fedora 44 container."""

import errno
import os
from pathlib import Path
import pty
import secrets
import select
import signal
import subprocess
import sys
import time

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
import firstboot


class Console:
    def __init__(self):
        self.pid, self.fd = pty.fork()
        if self.pid == 0:
            os.environ["LC_ALL"] = "C"
            os.environ["TERM"] = "dumb"
            os.execv(sys.executable, [sys.executable, str(Path(firstboot.__file__))])
        self.buffer = b""

    def expect(self, text):
        expected = text.encode()
        deadline = time.monotonic() + 30
        while expected not in self.buffer:
            if time.monotonic() > deadline:
                raise AssertionError(f"Timed out waiting for {text!r}")
            if not select.select([self.fd], [], [], 0.1)[0]:
                continue
            try:
                data = os.read(self.fd, 65536)
            except OSError as error:
                if error.errno != errno.EIO:
                    raise
                data = b""
            if not data:
                # Never include a password-bearing terminal transcript in CI logs.
                raise AssertionError(f"Setup exited before {text!r}")
            self.buffer += data
        self.buffer = self.buffer.split(expected, 1)[1]

    def answer(self, text, value):
        self.expect(text)
        os.write(self.fd, value.encode() + b"\n")

    def finish(self):
        deadline = time.monotonic() + 30
        while time.monotonic() < deadline:
            pid, status = os.waitpid(self.pid, os.WNOHANG)
            if pid:
                self.pid = None
                assert os.waitstatus_to_exitcode(status) == 0, "Setup failed"
                return
            time.sleep(0.05)
        raise AssertionError("Setup did not finish")

    def close(self):
        if self.pid is not None:
            os.killpg(self.pid, signal.SIGKILL)
            os.waitpid(self.pid, 0)
            self.pid = None
        os.close(self.fd)


def main():
    if os.environ.get("RYOKU_TEST_DISPOSABLE") != "1" or not (
        Path("/run/.containerenv").exists() or Path("/.dockerenv").exists()
    ):
        sys.exit("Run only in an explicitly disposable Fedora 44 container")
    assert os.geteuid() == 0
    assert 'VERSION_ID=44' in Path("/etc/os-release").read_text() or 'VERSION_ID="44"' in Path("/etc/os-release").read_text()
    subprocess.run(["rpm", "-q", "systemd", "shadow-utils", "glibc", "kbd"], check=True)
    stock = Path("/usr/lib/systemd/system/systemd-firstboot.service").read_text()
    assert "--prompt-hostname" not in stock, "Reaudit the changed Fedora stock unit"
    assert "--prompt-root-password" in stock
    subprocess.run(["useradd", "--create-home", "--groups", "wheel", "--shell", "/usr/bin/fish", "ryoku"], check=True)
    subprocess.run(["usermod", "--password", "!", "root"], check=True)
    for _, relative in firstboot.SETTINGS:
        (Path("/") / relative).unlink(missing_ok=True)
    state = Path("/") / firstboot.STATE
    state.mkdir(mode=0o700, parents=True)
    (state / "armed").write_text("1\n")
    root_password = secrets.token_urlsafe(24)
    user_password = secrets.token_urlsafe(24)
    console = Console()
    try:
        console.answer("Please enter the new system locale name or number", "en_US.UTF-8")
        console.answer("Please enter the new system message locale name or number", "en_US.UTF-8")
        console.answer("Please enter the new keymap name or number", "us")
        console.answer("Please enter the new timezone name or number", "Europe/Vienna")
        console.answer("Please enter the new hostname", "ryoku-test")
        console.answer("Please enter the new root password (empty to skip):", root_password)
        console.answer("Please enter the new root password again:", root_password)
        console.expect("Choose the password for your ryoku login account.")
    finally:
        console.close()
    assert not (state / "complete").exists()
    assert firstboot.password_set(Path("/"), "root")
    assert not firstboot.password_set(Path("/"), "ryoku")
    root_entry = Path("/etc/shadow").read_text().splitlines()[0]
    console = Console()
    try:
        console.answer("New password:", user_password)
        console.answer("Retype new password:", user_password)
        console.finish()
    finally:
        console.close()
    assert (state / "complete").exists()
    assert firstboot.password_set(Path("/"), "ryoku")
    assert root_entry == Path("/etc/shadow").read_text().splitlines()[0]
    assert Path("/etc/hostname").read_text().strip() == "ryoku-test"
    assert "Europe/Vienna" in os.readlink("/etc/localtime")
    assert "LANG=en_US.UTF-8" in Path("/etc/locale.conf").read_text()
    assert "KEYMAP=us" in Path("/etc/vconsole.conf").read_text()
    subprocess.run([sys.executable, str(Path(firstboot.__file__))], check=True, stdin=subprocess.DEVNULL, timeout=10)
    print("Fedora 44 prompts, interruption, password preservation and completed rerun passed.")


if __name__ == "__main__":
    main()
