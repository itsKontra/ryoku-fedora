#!/usr/bin/env python3
"""Transform an offline Fedora 44 target into a configured Ryoku desktop."""

import argparse
import os
from pathlib import Path
import shutil
import subprocess
import sys


RYOKU_USER = "ryoku"
RYOKU_SHELL = "/usr/bin/fish"
RYOKU_HOME = "/home/ryoku"
REQUIRED_BINARIES = (
    "usr/bin/python3",
    "usr/bin/fish",
    "usr/bin/passwd",
    "usr/bin/systemd-firstboot",
)


def validate_target(root, allow_running=False):
    root = root.resolve(strict=True)
    if not allow_running and root == Path("/"):
        raise ValueError("Expected an offline installation target, not a running system")
    release = (root / "etc/os-release").read_text()
    values = dict(line.split("=", 1) for line in release.splitlines() if "=" in line)
    if values.get("ID", "").strip('"') != "fedora" or values.get("VERSION_ID", "").strip('"') != "44":
        raise ValueError("Expected a Fedora 44 installation target")
    for relative in REQUIRED_BINARIES:
        if not (root / relative).is_file():
            raise ValueError(f"Missing target dependency: {relative}")
    return root


def configure_accounts_and_sudo(root):
    passwd_path = root / "etc/passwd"
    group_path = root / "etc/group"
    shadow_path = root / "etc/shadow"

    passwd_lines = passwd_path.read_text().splitlines() if passwd_path.exists() else []
    accounts = dict((p[0], p) for p in (line.split(":") for line in passwd_lines if line))

    target_uid = 1000
    target_gid = 1000
    if RYOKU_USER in accounts:
        user = accounts[RYOKU_USER]
        target_uid = int(user[2])
        target_gid = int(user[3])
        if user[5:] != [RYOKU_HOME, RYOKU_SHELL]:
            # Normalize existing account so home and login shell meet contract
            new_lines = []
            for line in passwd_lines:
                parts = line.split(":")
                if parts[0] == RYOKU_USER:
                    parts[5] = RYOKU_HOME
                    parts[6] = RYOKU_SHELL
                    line = ":".join(parts)
                new_lines.append(line)
            passwd_path.write_text("\n".join(new_lines) + "\n")
    else:
        existing_uids = {int(p[2]) for p in accounts.values() if p[2].isdigit()}
        while target_uid in existing_uids:
            target_uid += 1
        target_gid = target_uid
        passwd_lines.append(f"{RYOKU_USER}:x:{target_uid}:{target_gid}:Ryoku Desktop:{RYOKU_HOME}:{RYOKU_SHELL}")
        passwd_path.write_text("\n".join(passwd_lines) + "\n")

    group_lines = group_path.read_text().splitlines() if group_path.exists() else []
    groups = dict((g[0], g) for g in (line.split(":") for line in group_lines if line))

    if RYOKU_USER not in groups:
        group_lines.append(f"{RYOKU_USER}:x:{target_gid}:")

    new_group_lines = []
    for line in group_lines:
        if not line:
            continue
        parts = line.split(":")
        if parts[0] == "wheel":
            members = [m for m in parts[3].split(",") if m]
            if RYOKU_USER not in members:
                members.append(RYOKU_USER)
                parts[3] = ",".join(members)
                line = ":".join(parts)
        new_group_lines.append(line)
    group_path.write_text("\n".join(new_group_lines) + "\n")

    shadow_lines = shadow_path.read_text().splitlines() if shadow_path.exists() else []
    shadow_users = dict((s[0], s) for s in (line.split(":") for line in shadow_lines if line))

    new_shadow_lines = []
    for line in shadow_lines:
        if not line:
            continue
        parts = line.split(":")
        if parts[0] == "root":
            if not parts[1]:
                parts[1] = "!"
                line = ":".join(parts)
        elif parts[0] == RYOKU_USER:
            if not parts[1]:
                parts[1] = "!"
                line = ":".join(parts)
        new_shadow_lines.append(line)

    if RYOKU_USER not in shadow_users:
        new_shadow_lines.append(f"{RYOKU_USER}:!::0:99999:7:::")

    shadow_path.write_text("\n".join(new_shadow_lines) + "\n")

    home_dir = root / RYOKU_HOME.lstrip("/")
    home_dir.mkdir(mode=0o700, parents=True, exist_ok=True)

    sudoers_d = root / "etc/sudoers.d"
    sudoers_d.mkdir(mode=0o750, parents=True, exist_ok=True)

    wheel_conf = sudoers_d / "10-ryoku-wheel"
    wheel_conf.write_text("%wheel ALL=(ALL:ALL) ALL\n")
    wheel_conf.chmod(0o440)

    for entry in sudoers_d.iterdir():
        if entry.is_file() and entry.name != "10-ryoku-wheel":
            try:
                content = entry.read_text()
                if "NOPASSWD" in content:
                    entry.unlink()
            except OSError:
                pass


def configure_session_and_greeter(root):
    conf_d = root / "etc/sddm.conf.d"
    conf_d.mkdir(mode=0o755, parents=True, exist_ok=True)

    greeter_wrapper = root / "usr/share/ryoku/lockscreen/ryoku-greeter"
    compositor = "/usr/share/ryoku/lockscreen/ryoku-greeter" if greeter_wrapper.is_file() else "weston --shell=kiosk"

    session_wrapper = root / "usr/share/ryoku/lockscreen/ryoku-wayland-session"
    session_cmd = (
        "/usr/share/ryoku/lockscreen/ryoku-wayland-session"
        if session_wrapper.is_file()
        else "/usr/share/sddm/scripts/wayland-session"
    )

    wayland_conf = conf_d / "10-ryoku-wayland.conf"
    wayland_conf.write_text(
        "[General]\n"
        "DisplayServer=wayland\n"
        "GreeterEnvironment=QT_QPA_PLATFORM=wayland,XCURSOR_THEME=Bibata-Modern-Ice,XCURSOR_SIZE=24,QML_XHR_ALLOW_FILE_READ=1\n\n"
        "[Wayland]\n"
        f"CompositorCommand={compositor}\n"
        f"SessionCommand={session_cmd}\n"
    )
    wayland_conf.chmod(0o644)

    session_conf = conf_d / "99-ryoku.conf"
    session_conf.write_text(
        "[Theme]\n"
        "Current=ryoku\n\n"
        "[Autologin]\n"
        "Session=niri.desktop\n"
    )
    session_conf.chmod(0o644)

    cursor_dir = root / "usr/share/icons/default"
    cursor_theme = cursor_dir / "index.theme"
    if not cursor_theme.exists():
        cursor_dir.mkdir(mode=0o755, parents=True, exist_ok=True)
        cursor_theme.write_text(
            "[Icon Theme]\n"
            "Name=Default\n"
            "Comment=Ryoku default cursor\n"
            "Inherits=Bibata-Modern-Ice\n"
        )
        cursor_theme.chmod(0o644)

    themes_dir = root / "usr/share/sddm/themes"
    ryoku_theme = themes_dir / "ryoku"
    compat_theme = themes_dir / "sddm-theme-ryoku"
    if ryoku_theme.is_dir() and not compat_theme.exists() and not compat_theme.is_symlink():
        compat_theme.symlink_to("ryoku")

    pam_sddm = root / "etc/pam.d/sddm"
    if pam_sddm.is_file():
        lines = pam_sddm.read_text().splitlines()
        content = "\n".join(lines)
        if "pam_gnome_keyring.so" not in content:
            new_lines = []
            for line in lines:
                new_lines.append(line)
                if line.strip().startswith("auth") and "password-auth" in line:
                    new_lines.append("-auth optional pam_gnome_keyring.so")
                elif line.strip().startswith("session") and "postlogin" in line:
                    new_lines.append("-session optional pam_gnome_keyring.so auto_start")
            pam_sddm.write_text("\n".join(new_lines) + "\n")

    if shutil.which("systemctl"):
        subprocess.run(["systemctl", f"--root={root}", "enable", "sddm.service"], check=False, stderr=subprocess.DEVNULL)
        subprocess.run(["systemctl", f"--root={root}", "set-default", "graphical.target"], check=False, stderr=subprocess.DEVNULL)

    system_units = root / "etc/systemd/system"
    system_units.mkdir(parents=True, exist_ok=True)
    dm_link = system_units / "display-manager.service"
    if not dm_link.exists() and not dm_link.is_symlink():
        dm_link.symlink_to("/usr/lib/systemd/system/sddm.service")
    target_link = system_units / "default.target"
    if not target_link.exists() and not target_link.is_symlink():
        target_link.symlink_to("/usr/lib/systemd/system/graphical.target")


def enable_base_services(root):
    services = ["NetworkManager.service", "firewalld.service", "bluetooth.service"]
    if shutil.which("systemctl"):
        subprocess.run(["systemctl", f"--root={root}", "enable", *services], check=False, stderr=subprocess.DEVNULL)

    multi_wants = root / "etc/systemd/system/multi-user.target.wants"
    multi_wants.mkdir(parents=True, exist_ok=True)
    for service in ("NetworkManager.service", "firewalld.service"):
        link = multi_wants / service
        if not link.exists() and not link.is_symlink():
            link.symlink_to(f"/usr/lib/systemd/system/{service}")

    bt_wants = root / "etc/systemd/system/bluetooth.target.wants"
    bt_wants.mkdir(parents=True, exist_ok=True)
    bt_link = bt_wants / "bluetooth.service"
    if not bt_link.exists() and not bt_link.is_symlink():
        bt_link.symlink_to("/usr/lib/systemd/system/bluetooth.service")


def seed_lockscreen(root, repo_dir=None, home=RYOKU_HOME):
    candidates = [
        root / "usr/share/ryoku/lockscreen/qylock",
    ]
    if repo_dir:
        candidates.append(Path(repo_dir) / "ryoku/lockscreen/qylock")
    candidates.append(Path(__file__).resolve().parents[2] / "ryoku/lockscreen/qylock")

    bundle = next((p for p in candidates if (p / "themes/clockwork/orbital/Main.qml").is_file()), None)
    if not bundle:
        return

    user_home = root / home.lstrip("/")
    lock_dir = user_home / ".local/share/quickshell-lockscreen"
    lock_dir.parent.mkdir(parents=True, exist_ok=True)

    src_lock = bundle / "quickshell-lockscreen"
    if src_lock.is_dir():
        if lock_dir.is_dir():
            shutil.rmtree(lock_dir)
        shutil.copytree(src_lock, lock_dir, dirs_exist_ok=True, symlinks=True)
        lock_sh = lock_dir / "lock.sh"
        if lock_sh.exists():
            lock_sh.chmod(0o755)

        compat_dir = root / "usr/lib64/qt6/qml/Qt5Compat"
        imports_dir = lock_dir / "imports"
        if compat_dir.is_dir() and imports_dir.is_dir():
            for link in imports_dir.rglob("*"):
                if link.is_symlink():
                    target = os.readlink(link)
                    if target.startswith("/usr/lib/qt6/qml/"):
                        rel_qt = target[len("/usr/lib/qt6/qml/"):]
                        link.unlink()
                        link.symlink_to(f"/usr/lib64/qt6/qml/{rel_qt}")

    themes_dir = user_home / ".local/share/qylock/themes"
    orbital_dir = themes_dir / "clockwork/orbital"
    orbital_dir.parent.mkdir(parents=True, exist_ok=True)

    src_orbital = bundle / "themes/clockwork/orbital"
    if src_orbital.is_dir():
        if orbital_dir.is_dir():
            shutil.rmtree(orbital_dir)
        shutil.copytree(src_orbital, orbital_dir, dirs_exist_ok=True, symlinks=True)

    themes_link = lock_dir / "themes_link"
    themes_link.unlink(missing_ok=True)
    themes_link.symlink_to("../qylock/themes")

    qylock_conf = user_home / ".config/qylock"
    qylock_conf.mkdir(parents=True, exist_ok=True)
    pref = qylock_conf / "theme"
    if not pref.exists():
        pref.write_text("clockwork/orbital\n")
        pref.chmod(0o644)


def seed_assets_and_integration(root, repo_dir=None, home=RYOKU_HOME):
    user_home = root / home.lstrip("/")
    repo = Path(repo_dir) if repo_dir else Path(__file__).resolve().parents[2]

    # 1. Desktop entries and MIME defaults
    apps_dir = root / "usr/share/applications"
    apps_dir.mkdir(parents=True, exist_ok=True)
    niri_mime = apps_dir / "niri-mimeapps.list"
    std_mime = apps_dir / "mimeapps.list"

    mime_source = None
    for src in (root / "usr/share/ryoku/apps/mimeapps.list", repo / "ryoku/apps/mimeapps.list"):
        if src.is_file():
            mime_source = src
            break

    if not niri_mime.exists() and mime_source:
        shutil.copyfile(mime_source, niri_mime)
        niri_mime.chmod(0o644)

    if not std_mime.exists() and not std_mime.is_symlink():
        if niri_mime.exists():
            std_mime.symlink_to("niri-mimeapps.list")
        elif mime_source:
            shutil.copyfile(mime_source, std_mime)
            std_mime.chmod(0o644)

    # 2. Wallpapers
    wallpapers_src = next(
        (p for p in (root / "usr/share/ryoku/wallpapers", repo / "ryoku/assets/wallpapers") if p.is_dir()),
        None,
    )
    if wallpapers_src:
        wallpapers_dst = user_home / "Pictures/Wallpapers"
        shutil.copytree(wallpapers_src, wallpapers_dst, dirs_exist_ok=True, symlinks=True)

    # 3. Ryodecors
    ryodecors_src = next(
        (p for p in (root / "usr/share/ryoku/ryodecors", repo / "ryoku/assets/ryodecors") if p.is_dir()),
        None,
    )
    if ryodecors_src:
        ryodecors_dst = user_home / "Pictures/ryodecors"
        shutil.copytree(ryodecors_src, ryodecors_dst, dirs_exist_ok=True, symlinks=True)

    # 4. Brand assets
    brand_src = next(
        (p for p in (root / "usr/share/ryoku/brand", repo / "ryoku/assets/brand") if p.is_dir()),
        None,
    )
    if brand_src:
        brand_dst = user_home / ".local/share/ryoku/assets/brand"
        shutil.copytree(brand_src, brand_dst, dirs_exist_ok=True, symlinks=True)

    # 5. npmrc
    npmrc_src = next(
        (p for p in (root / "usr/share/ryoku/config/npm/npmrc", repo / "ryoku/apps/npm/npmrc") if p.is_file()),
        None,
    )
    if npmrc_src:
        npmrc_dst = user_home / ".npmrc"
        shutil.copyfile(npmrc_src, npmrc_dst)
        npmrc_dst.chmod(0o644)


def materialize_config(root, runner=None, user=RYOKU_USER, home=RYOKU_HOME):
    ryoku_bin = root / "usr/bin/ryoku"
    user_home = root / home.lstrip("/")

    if runner:
        runner([
            "chroot",
            str(root),
            "runuser",
            "-u",
            user,
            "--",
            "env",
            f"HOME={home}",
            f"USER={user}",
            f"LOGNAME={user}",
            "ryoku",
            "materialize",
        ])
    elif ryoku_bin.is_file() and (root / "usr/bin/runuser").is_file() and (root / "lib64/libc.so.6").is_file() and shutil.which("chroot"):
        try:
            subprocess.run(
                [
                    "chroot",
                    str(root),
                    "runuser",
                    "-u",
                    user,
                    "--",
                    "env",
                    f"HOME={home}",
                    f"USER={user}",
                    f"LOGNAME={user}",
                    "ryoku",
                    "materialize",
                ],
                check=True,
            )
        except Exception:
            if (root / "usr/share/ryoku/config").is_dir():
                shutil.copytree(root / "usr/share/ryoku/config", user_home / ".config", dirs_exist_ok=True, symlinks=True)
    elif (root / "usr/share/ryoku/config").is_dir():
        shutil.copytree(root / "usr/share/ryoku/config", user_home / ".config", dirs_exist_ok=True, symlinks=True)

    ryoku_cfg = user_home / ".config/ryoku"
    ryoku_cfg.mkdir(parents=True, exist_ok=True)
    desktop_json = ryoku_cfg / "desktop.json"
    if not desktop_json.exists():
        desktop_json.write_text('{"desktop":{},"wm":{"niri":{}}}\n')

    niri_bin = root / "usr/bin/ryoku-wm-niri"
    if niri_bin.is_file():
        if runner:
            runner([
                "chroot",
                str(root),
                "runuser",
                "-u",
                user,
                "--",
                "env",
                f"HOME={home}",
                f"USER={user}",
                f"LOGNAME={user}",
                "XDG_CURRENT_DESKTOP=niri",
                "/usr/bin/ryoku-wm-niri",
                "apply",
                f"{home}/.config/ryoku/desktop.json",
            ])
        elif (root / "usr/bin/runuser").is_file() and (root / "lib64/libc.so.6").is_file() and shutil.which("chroot"):
            try:
                subprocess.run(
                    [
                        "chroot",
                        str(root),
                        "runuser",
                        "-u",
                        user,
                        "--",
                        "env",
                        f"HOME={home}",
                        f"USER={user}",
                        f"LOGNAME={user}",
                        "XDG_CURRENT_DESKTOP=niri",
                        "/usr/bin/ryoku-wm-niri",
                        "apply",
                        f"{home}/.config/ryoku/desktop.json",
                    ],
                    check=True,
                )
            except Exception:
                pass

    niri_dir = user_home / ".config/niri"
    if niri_dir.is_dir():
        for gen in ("settings.kdl", "rebinds.kdl"):
            gen_path = niri_dir / gen
            if not gen_path.exists():
                gen_path.touch()

    hypr_dir = user_home / ".config/hypr"
    if hypr_dir.is_dir():
        for gen in ("settings.lua", "rebinds.lua"):
            gen_path = hypr_dir / gen
            if not gen_path.exists():
                gen_path.touch()

    passwd_path = root / "etc/passwd"
    if passwd_path.exists() and os.geteuid() == 0:
        accounts = dict((p[0], p) for p in (line.split(":") for line in passwd_path.read_text().splitlines() if line))
        if user in accounts:
            uid = int(accounts[user][2])
            gid = int(accounts[user][3])
            for path in user_home.rglob("*"):
                try:
                    os.lchown(path, uid, gid)
                except OSError:
                    pass
            try:
                os.lchown(user_home, uid, gid)
            except OSError:
                pass


def arm_firstboot(root, runner=None, allow_running=False):
    prepare_script = Path(__file__).resolve().parent / "prepare-firstboot.py"
    cmd = [sys.executable, str(prepare_script)]
    if allow_running:
        cmd.append("--allow-running-system")
    cmd.append(str(root))
    if runner:
        runner(cmd)
    else:
        subprocess.run(cmd, check=True)


def relabel_selinux(root, runner=None, home=RYOKU_HOME):
    contexts = root / "etc/selinux/targeted/contexts/files/file_contexts"
    if not contexts.is_file():
        return

    targets = [
        str(root / "etc"),
        str(root / "var"),
        str(root / home.lstrip("/")),
    ]
    if runner:
        runner(["setfiles", "-r", str(root), str(contexts), *targets])
    elif shutil.which("setfiles"):
        subprocess.run(["setfiles", "-r", str(root), str(contexts), *targets], check=True)
    elif shutil.which("chroot") and (root / "usr/sbin/restorecon").is_file():
        subprocess.run(["chroot", str(root), "restorecon", "-Rv", "/etc", "/var", home], check=True)


def anaconda_accounts(root):
    """Use the accounts created by Anaconda without rewriting credentials."""
    accounts = []
    for line in (root / "etc/passwd").read_text().splitlines():
        fields = line.split(":")
        if len(fields) == 7 and 1000 <= int(fields[2]) < 65534 and fields[6] not in (
            "/sbin/nologin", "/usr/sbin/nologin", "/bin/false",
        ):
            home = Path(fields[5])
            if not home.is_absolute() or ".." in home.parts or home == Path("/"):
                raise ValueError("Invalid installation account home")
            accounts.append((fields[0], str(home)))
    if not accounts:
        raise ValueError("Create a login account in Anaconda User Creation before installing")
    sudoers = root / "etc/sudoers.d/10-ryoku-wheel"
    sudoers.parent.mkdir(parents=True, exist_ok=True)
    sudoers.write_text("%wheel ALL=(ALL:ALL) ALL\n")
    sudoers.chmod(0o440)
    return accounts


def provision(root, repo_dir=None, runner=None, allow_running=False, anaconda=False):
    root = validate_target(root, allow_running=allow_running)
    if anaconda:
        accounts = anaconda_accounts(root)
    else:
        configure_accounts_and_sudo(root)
        accounts = [(RYOKU_USER, RYOKU_HOME)]
    configure_session_and_greeter(root)
    enable_base_services(root)
    for user, home in accounts:
        seed_assets_and_integration(root, repo_dir=repo_dir, home=home)
        seed_lockscreen(root, repo_dir=repo_dir, home=home)
        materialize_config(root, runner=runner, user=user, home=home)
    if not anaconda:
        if allow_running:
            arm_firstboot(root, runner=runner, allow_running=True)
        else:
            arm_firstboot(root, runner=runner)
    for _, home in accounts:
        relabel_selinux(root, runner=runner, home=home)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("root", type=Path, help="mounted target root after package installation")
    parser.add_argument("--repo", type=Path, default=None, help="optional path to Ryoku repository")
    parser.add_argument(
        "--allow-running-system",
        action="store_true",
        help="allow execution against a running system (test use only)",
    )
    parser.add_argument("--anaconda", action="store_true", help="preserve accounts and regional settings configured in Anaconda")
    args = parser.parse_args()

    try:
        if os.geteuid() != 0:
            raise ValueError("Provisioning an installation target requires root")
        provision(args.root, repo_dir=args.repo, allow_running=args.allow_running_system, anaconda=args.anaconda)
    except (OSError, ValueError, subprocess.CalledProcessError) as error:
        sys.exit(str(error))


if __name__ == "__main__":
    main()
