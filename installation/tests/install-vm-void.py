#!/usr/bin/env python3
"""Install the Ryoku Void ISO over serial, then require a booted login prompt."""
import base64
import argparse
import os
import shlex
import shutil
import re
import subprocess
import sys
import tempfile

import pexpect


def first_existing(paths):
    return next((path for path in paths if os.path.exists(path)), None)


OVMF_CODE = first_existing((
    "/usr/share/edk2/x64/OVMF_CODE.4m.fd",
    "/usr/share/edk2-ovmf/x64/OVMF_CODE.4m.fd",
    "/usr/share/OVMF/OVMF_CODE_4M.fd",
    "/usr/share/OVMF/OVMF_CODE.fd",
))
OVMF_VARS = first_existing((
    "/usr/share/edk2/x64/OVMF_VARS.4m.fd",
    "/usr/share/edk2-ovmf/x64/OVMF_VARS.4m.fd",
    "/usr/share/OVMF/OVMF_VARS_4M.fd",
    "/usr/share/OVMF/OVMF_VARS.fd",
))
OFFLINE_REPO = "/run/initramfs/live/ryoku/repo"
LIVE_CSI_PREFIX = r"(?:\x1b\[[0-9;?]*[A-Za-z])*"
LIVE_LOGIN_RE = rf"(?m)^{LIVE_CSI_PREFIX}[^\r\n]* login: ?$"
LIVE_SHELL_RE = rf"(?m)^{LIVE_CSI_PREFIX}-?bash-[0-9][0-9.]*# $"
LIVE_PROMPT_RE = rf"(?m)^{LIVE_CSI_PREFIX}RYOKU_LIVE# $"
LIVE_SYNC_RE = rf"(?m)^{LIVE_CSI_PREFIX}RYOKU_SYNC_42\r*$"


def qemu_command(work, iso=None, bios=False):
    accel = (["-enable-kvm", "-cpu", "host"]
             if os.path.exists("/dev/kvm") else ["-cpu", "max"])
    command = ["qemu-system-x86_64", "-machine", "q35"]
    if not bios:
        vars_copy = os.path.join(work, "OVMF_VARS.fd")
        if not os.path.exists(vars_copy):
            shutil.copy2(OVMF_VARS, vars_copy)
        command += [
            "-drive", f"if=pflash,format=raw,readonly=on,file={OVMF_CODE}",
            "-drive", f"if=pflash,format=raw,file={vars_copy}",
        ]
    command += [
        *accel,
        "-m", "4096", "-smp", "4",
        "-drive", f"file={os.path.join(work, 'target.qcow2')},if=virtio,format=qcow2",
        "-netdev", "user,id=n0", "-device", "virtio-net-pci,netdev=n0",
        "-nographic",
    ]
    if iso:
        command += ["-drive", f"file={os.path.abspath(iso)},media=cdrom,readonly=on", "-boot", "d"]
    else:
        command += ["-boot", "c"]
    return command


def serial_tail(path):
    try:
        with open(path, encoding="utf-8", errors="replace") as stream:
            return stream.read()[-5000:]
    except OSError as error:
        return f"could not read serial log: {error}"


def spawn(command, timeout, log_path, mode):
    child = pexpect.spawn(command[0], command[1:], encoding="utf-8",
                          codec_errors="replace", timeout=timeout)
    child.logfile = open(log_path, mode, encoding="utf-8")
    return child


def _reach_live_shell(child, timeout):
    choice = child.expect(
        [LIVE_LOGIN_RE, LIVE_SHELL_RE, LIVE_PROMPT_RE], timeout=timeout)
    if choice != 0:
        return
    automatic = child.expect(
        [r"\(automatic login\)", pexpect.TIMEOUT], timeout=2)
    if automatic == 0:
        child.expect(LIVE_SHELL_RE, timeout=20)
        return
    child.sendline("root")
    choice = child.expect([r"(?m)^Password: ?$", LIVE_SHELL_RE], timeout=20)
    if choice == 0:
        child.sendline("voidlinux")
        child.expect(LIVE_SHELL_RE, timeout=20)


def reach_live_root(child):
    sync_command = (
        "bind 'set enable-bracketed-paste off' 2>/dev/null; "
        "export PS1='RYOKU_LIVE# '; echo RYOKU_SYNC_$((40+2))"
    )
    for attempt in range(6):
        try:
            if attempt:
                # A respawned getty may be sitting silently at login or at a
                # fresh shell prompt. A blank line makes either state visible.
                child.sendline("")
            _reach_live_shell(child, 300 if attempt == 0 else 20)
            child.sendline(sync_command)
            child.expect(LIVE_SYNC_RE, timeout=15)
            child.expect(LIVE_PROMPT_RE, timeout=15)
            return
        except pexpect.TIMEOUT:
            continue
        except pexpect.EOF as error:
            raise RuntimeError(
                "live serial getty exited during prompt sync") from error
    raise RuntimeError("live ISO did not produce a stable serial root shell")


def shutdown(child):
    child.sendline("poweroff")
    try:
        child.expect(pexpect.EOF, timeout=180)
    finally:
        child.close(force=True)


def run_live_check(child, name, command, timeout=120):
    marker = "USERS_CHECK_" + re.sub(r"[^A-Za-z0-9]", "_", name).upper()
    child.sendline(f"{command}; check_status=$?; echo {marker}:$check_status")
    child.expect(rf"{marker}:([0-9]+)", timeout=timeout)
    status = int(child.match.group(1))
    child.expect(LIVE_PROMPT_RE, timeout=60)
    if status != 0:
        raise RuntimeError(f"account assertion failed: {name}")
    print(f"install-vm-void: account assertion OK: {name}")



def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--iso", required=True, help="Ryoku Void ISO")
    parser.add_argument("--work", help="persistent work directory")
    parser.add_argument("--user", default="test")
    parser.add_argument("--extra-admin", default="admin2")
    parser.add_argument("--extra-standard", default="standard")
    parser.add_argument("--standard-limit-bytes", type=int, default=1073741824)
    parser.add_argument("--timeout", type=int, default=3600,
                        help="backend timeout in seconds (default: 3600)")
    parser.add_argument("--boot-timeout", type=int, default=600,
                        help="installed boot timeout in seconds (default: 600)")
    parser.add_argument("--bios", action="store_true",
                        help="boot the live and installed systems with SeaBIOS")
    args = parser.parse_args()
    account_names = (args.user, args.extra_admin, args.extra_standard)
    account_name_re = re.compile(r"^[a-z_][a-z0-9_-]{0,31}$")
    if any(not account_name_re.fullmatch(name) for name in account_names):
        parser.error("account names must match ^[a-z_][a-z0-9_-]{0,31}$")
    if len(set(account_names)) != len(account_names):
        parser.error("primary, extra administrator, and standard account names must differ")
    if args.standard_limit_bytes <= 0 or args.standard_limit_bytes > (1 << 63) - 1:
        parser.error("--standard-limit-bytes must be between 1 and 9223372036854775807")


    if not args.bios and (not OVMF_CODE or not OVMF_VARS):
        parser.error("OVMF firmware was not found")
    for command in ("qemu-system-x86_64", "qemu-img", "openssl"):
        if not shutil.which(command):
            parser.error(f"required command not found: {command}")

    work = os.path.abspath(args.work or tempfile.mkdtemp(prefix="ryoku-void-vm-"))
    os.makedirs(work, exist_ok=True)
    vars_copy = os.path.join(work, "OVMF_VARS.fd")
    if os.path.exists(vars_copy):
        os.remove(vars_copy)
    disk = os.path.join(work, "target.qcow2")
    if os.path.exists(disk):
        os.remove(disk)
    subprocess.run(["qemu-img", "create", "-f", "qcow2", disk, "40G"], check=True)
    password_hash = subprocess.check_output(
        ["openssl", "passwd", "-6", "test"], text=True).strip()
    extra_users_file = "/tmp/ryoku-extra-users.tsv"
    extra_users = "\n".join((
        f"{args.extra_admin}\tAdditional Administrator\tadmin\tdevices\t{password_hash}\t0",
        f"{args.extra_standard}\tStandard User\tstandard\t\t{password_hash}\t"
        f"{args.standard_limit_bytes}",
    )) + "\n"
    log_path = os.path.join(work, "serial.log")

    child = spawn(qemu_command(work, args.iso, args.bios), args.timeout, log_path, "w")
    try:
        reach_live_root(child)
        child.sendline(
            f"test -r {OFFLINE_REPO}/x86_64-repodata && "
            f"ls {OFFLINE_REPO}/*.xbps >/dev/null 2>&1; echo REPO_READY:$?")
        child.expect(r"REPO_READY:0", timeout=60)
        child.expect(LIVE_PROMPT_RE, timeout=60)
        extra_users_b64 = base64.b64encode(extra_users.encode()).decode()
        child.sendline(
            f"printf %s {shlex.quote(extra_users_b64)} | base64 -d > "
            f"{shlex.quote(extra_users_file)}"
            f" && chmod 0600 {shlex.quote(extra_users_file)}"
            " && echo EXTRA_USERS_READY")
        child.expect(r"EXTRA_USERS_READY", timeout=30)
        child.expect(LIVE_PROMPT_RE, timeout=30)


        environment = {
            "RYOKU_DISK": "/dev/vda",
            "RYOKU_DISK_STRATEGY": "whole",
            "RYOKU_WIPE_CONFIRMED": "1",
            "RYOKU_HOSTNAME": "ryoku-test",
            "RYOKU_USERNAME": args.user,
            "RYOKU_FULLNAME": "Primary Administrator",
            "RYOKU_ROOT_MODE": "locked",
            "RYOKU_EXTRA_USERS_FILE": extra_users_file,
            "RYOKU_PASSWORD_HASH": password_hash,
            "RYOKU_COMPOSITOR": "niri",
            "RYOKU_COMPOSITOR_CONFIG_DIR": "niri",
            "RYOKU_BROWSER": "firefox",
            "RYOKU_LOGIN_SHELL": "fish",
            "RYOKU_PROFILE": "vm",
            "RYOKU_ONLINE": "0",
            "RYOKU_OFFLINE_REPO": OFFLINE_REPO,
            "RYOKU_REPO": "/usr/share/ryoku",
            "RYOKU_ENABLE_SERIAL": "1",
            "RYOKU_KERNEL_CMDLINE_EXTRA": "console=ttyS0,115200n8",
        }
        exports = " ".join(
            f"{name}={shlex.quote(value)}" for name, value in environment.items())
        child.sendline(f"export {exports}; ryoku-install; echo BACKEND_EXIT:$?")
        result = child.expect(
            [r"@@RYOKU_DONE", r"BACKEND_EXIT:[1-9][0-9]*", pexpect.TIMEOUT],
            timeout=args.timeout)
        if result != 0:
            raise RuntimeError("backend did not emit @@RYOKU_DONE")
        child.expect(r"BACKEND_EXIT:0", timeout=180)
        child.expect(LIVE_PROMPT_RE, timeout=60)
        run_live_check(
            child, "installed target remount",
            "mkdir -p /mnt"
            " && mount -o subvol=@ /dev/vda2 /mnt"
            " && mkdir -p /mnt/home"
            " && mount -o subvol=@home /dev/vda2 /mnt/home")
        account_shell_names = " ".join(shlex.quote(name) for name in account_names)
        run_live_check(
            child, "passwd entries",
            " && ".join(
                f"grep -q '^{name}:' /mnt/etc/passwd" for name in account_names))
        run_live_check(
            child, "wheel administrators only",
            "IFS=: read -r _ _ _ members < <(grep '^wheel:' /mnt/etc/group)"
            f" && case \",$members,\" in *',{args.user},'*) ;; *) false;; esac"
            f" && case \",$members,\" in *',{args.extra_admin},'*) ;; *) false;; esac"
            f" && case \",$members,\" in *',{args.extra_standard},'*) false;; esac"
            " && IFS=, read -ra wheel_users <<<\"$members\""
            f" && for wheel_user in \"${{wheel_users[@]}}\"; do"
            f" [[ $wheel_user == {shlex.quote(args.user)}"
            f" || $wheel_user == {shlex.quote(args.extra_admin)} ]]"
            " || exit 1; done")
        run_live_check(
            child, "standard account device access",
            "for group in input video; do "
            "IFS=: read -r _ _ _ members < <(grep \"^$group:\" /mnt/etc/group)"
            f" && case \",$members,\" in *',{args.extra_standard},'*) ;; *) exit 1;; esac"
            " || exit 1; done")
        run_live_check(
            child, "root locked",
            "IFS=: read -r _ root_password _ < <(grep '^root:' /mnt/etc/shadow)"
            " && [[ $root_password == '!'* ]]")
        run_live_check(
            child, "private user-owned Btrfs homes",
            f"for account in {account_shell_names}; do "
            "IFS=: read -r _ _ uid gid _ _ _ < <(grep \"^$account:\" /mnt/etc/passwd)"
            " && [[ $(stat -c '%u:%g:%a' \"/mnt/home/$account\") == \"$uid:$gid:700\" ]]"
            " && btrfs subvolume show \"/mnt/home/$account\" >/dev/null"
            " || exit 1; done")
        run_live_check(
            child, "standard qgroup limit",
            "read -r _ _ subvol_id _ < <(btrfs subvolume show "
            f"/mnt/home/{shlex.quote(args.extra_standard)} | grep 'Subvolume ID:')"
            " && qgroup_found=1"
            " && while read -r qgroup rfer excl max_rfer max_excl; do "
            f"[[ $qgroup == \"0/$subvol_id\" && $max_rfer == "
            f"{args.standard_limit_bytes} ]] && qgroup_found=0; done "
            f"< <(btrfs qgroup show --raw -reF /mnt/home/{shlex.quote(args.extra_standard)})"
            " && ((qgroup_found == 0))")
        child.sendline(
            f"echo USERS_ASSERTIONS_PASS:{args.user},"
            f"{args.extra_admin},{args.extra_standard}")
        child.expect(
            rf"USERS_ASSERTIONS_PASS:{re.escape(args.user)},"
            rf"{re.escape(args.extra_admin)},{re.escape(args.extra_standard)}",
            timeout=30)
        child.expect(LIVE_PROMPT_RE, timeout=30)
        run_live_check(child, "installed target unmount", "umount -R /mnt")
        shutdown(child)
    except (pexpect.TIMEOUT, pexpect.EOF, RuntimeError) as error:
        child.close(force=True)
        print(f"install-vm-void: {error}\n--- serial tail ---\n{serial_tail(log_path)}",
              file=sys.stderr)
        return 1

    boot_log = os.path.join(work, "installed-serial.log")
    installed = spawn(qemu_command(work, bios=args.bios), args.boot_timeout, boot_log, "w")
    try:
        installed.expect(r"(?m)^ryoku-test login: ?$", timeout=args.boot_timeout)
        print("install-vm-void: installed Void system reached the serial login prompt")
        installed.close(force=True)
    except (pexpect.TIMEOUT, pexpect.EOF) as error:
        installed.close(force=True)
        print(f"install-vm-void: installed boot failed: {error}\n--- serial tail ---\n"
              f"{serial_tail(boot_log)}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
