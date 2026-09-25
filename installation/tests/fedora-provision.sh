#!/usr/bin/env bash
set -euo pipefail
[[ ${RYOKU_TEST_DISPOSABLE:-0} == 1 && $EUID == 0 && -f /etc/fedora-release ]] || {
  echo 'requires an explicitly disposable Fedora 44 container' >&2
  exit 1
}
[[ -f /run/.containerenv || -f /.dockerenv ]] || {
  echo 'refusing to run outside a disposable container' >&2
  exit 1
}

root=$(cd "$(dirname "$0")/../.." && pwd)

# 1. Run unit tests
python3 -m unittest discover -s "$root/installation/fedora/tests" -v

# 2. Prepare isolated target sysroot
target=$(mktemp -d /tmp/target-sysroot.XXXXXX)
trap 'rm -rf "$target"' EXIT
chmod 755 "$target"

mkdir -p "$target"/{etc,usr/bin,usr/sbin,usr/share,var/lib,home,run}
mkdir -p "$target"/etc/{pam.d,sudoers.d,systemd/system,sddm.conf.d}
mkdir -p "$target"/usr/lib/systemd/system
mkdir -p "$target"/usr/share/{ryoku,applications,icons,sddm/themes/ryoku}

cp /etc/os-release "$target/etc/os-release"

echo "root:x:0:0:root:/root:/bin/bash" > "$target/etc/passwd"
echo "root:x:0:" > "$target/etc/group"
echo "wheel:x:10:" >> "$target/etc/group"
echo "root:!::0:99999:7:::" > "$target/etc/shadow"
echo -e "auth substack password-auth\nsession include postlogin" > "$target/etc/pam.d/sddm"

# Copy required host binaries and systemd units into the offline target
cp -a /usr/lib/systemd/system/. "$target/usr/lib/systemd/system/"

for bin in python3 fish passwd systemd-firstboot chroot; do
  loc=$(type -p "$bin" || true)
  if [[ -n $loc && -f $loc ]]; then
    install -Dm755 "$loc" "$target/usr/bin/$bin"
  fi
done

# Exercise chroot and runuser with real binaries and PAM, including their libraries.
copy_runtime() {
  local binary=$1 library
  cp -L --parents "$binary" "$target"
  while read -r library; do
    [[ -f $library ]] && cp -L --parents "$library" "$target"
  done < <(ldd "$binary" | awk '/=> \// {print $3} /^[[:space:]]*\// {print $1}')
}
for binary in sh runuser env id mkdir cp test true; do
  copy_runtime "/usr/bin/$binary"
done
ln -s usr/bin "$target/bin"
for module in pam_rootok pam_permit; do
  copy_runtime "/usr/lib64/security/$module.so"
done
cat > "$target/etc/pam.d/runuser" <<'EOF'
auth sufficient pam_rootok.so
account required pam_permit.so
session required pam_permit.so
EOF

cat > "$target/usr/bin/ryoku" <<'EOF'
#!/bin/sh
set -eu
[ "$(id -u)" = 1000 ]
[ "$(id -g)" = 1000 ]
[ "$1" = materialize ]
mkdir -p "$HOME/.config/ryoku" "$HOME/.local/state/ryoku"
cp -r /usr/share/ryoku/config/. "$HOME/.config/"
echo materialized > "$HOME/.local/state/ryoku/materialized"
EOF
cat > "$target/usr/bin/ryoku-wm-hyprland" <<'EOF'
#!/bin/sh
set -eu
[ "$(id -u)" = 1000 ]
[ "$1" = apply ]
[ -s "$2" ]
[ -s "$HOME/.local/state/ryoku/materialized" ]
mkdir -p "$HOME/.config/hypr"
echo 'return {}' > "$HOME/.config/hypr/settings.lua"
echo 'return {}' > "$HOME/.config/hypr/rebinds.lua"
EOF
chmod 755 "$target/usr/bin/ryoku" "$target/usr/bin/ryoku-wm-hyprland"

# Model extras already supplied by the package payload, with no network needed.
for asset in share/icons/Bibata-Modern-Ice/cursors/left_ptr \
  share/fonts/SpaceGrotesk/regular.otf share/fonts/MaterialSymbolsRounded.ttf \
  share/fonts/JetBrainsMonoNerdFont/regular.ttf \
  share/fonts/SpaceMonoNerdFont/regular.ttf bin/matugen; do
  install -Dm644 /etc/hostname "$target/usr/$asset"
done

# Create minimal service units for base services
cat > "$target/usr/lib/systemd/system/sddm.service" <<'EOF'
[Unit]
Description=Simple Desktop Display Manager
After=systemd-user-sessions.service
[Service]
ExecStart=/usr/bin/true
[Install]
Alias=display-manager.service
EOF

for unit in NetworkManager.service firewalld.service bluetooth.service; do
  cat > "$target/usr/lib/systemd/system/$unit" <<EOF
[Unit]
Description=$unit
[Service]
ExecStart=/usr/bin/true
[Install]
WantedBy=multi-user.target
EOF
done

# Copy payload assets from repository into target payload paths
mkdir -p "$target/usr/share/ryoku"
cp -a "$root/ryoku/assets/wallpapers" "$target/usr/share/ryoku/wallpapers"
cp -a "$root/ryoku/assets/ryodecors" "$target/usr/share/ryoku/ryodecors"
cp -a "$root/ryoku/assets/brand" "$target/usr/share/ryoku/brand"
mkdir -p "$target/usr/share/ryoku/lockscreen"
cp -a "$root/ryoku/lockscreen/qylock" "$target/usr/share/ryoku/lockscreen/qylock"
mkdir -p "$target/usr/share/ryoku/apps"
cp -a "$root/ryoku/apps/mimeapps.list" "$target/usr/share/ryoku/apps/mimeapps.list"
mkdir -p "$target/usr/share/ryoku/config/ryoku"
echo '{"desktop":{"input":{"kbLayout":"us"}}}' > "$target/usr/share/ryoku/config/ryoku/desktop.json"

# 3. Execute offline desktop provisioner
python3 "$root/installation/fedora/provision-target.py" "$target"

# 4. Verify accounts and sudo configuration
grep -q '^ryoku:x:1000:1000:Ryoku Desktop:/home/ryoku:/usr/bin/fish' "$target/etc/passwd"
grep -q '^wheel:.*ryoku' "$target/etc/group"
grep -q '^ryoku:!:' "$target/etc/shadow"
grep -q '^root:!:' "$target/etc/shadow"
test -f "$target/etc/sudoers.d/10-ryoku-wheel"
[[ $(stat -c %a "$target/etc/sudoers.d/10-ryoku-wheel") == "440" ]]

# 5. Verify SDDM session and greeter selection
test -f "$target/etc/sddm.conf.d/10-ryoku-wayland.conf"
grep -q 'DisplayServer=wayland' "$target/etc/sddm.conf.d/10-ryoku-wayland.conf"
test -f "$target/etc/sddm.conf.d/99-ryoku.conf"
grep -q 'Current=ryoku' "$target/etc/sddm.conf.d/99-ryoku.conf"
grep -q 'Session=hyprland.desktop' "$target/etc/sddm.conf.d/99-ryoku.conf"
[[ -L "$target/etc/systemd/system/display-manager.service" ]]
[[ $(readlink "$target/etc/systemd/system/display-manager.service") == "/usr/lib/systemd/system/sddm.service" ]]
[[ -L "$target/etc/systemd/system/default.target" ]]
[[ $(readlink "$target/etc/systemd/system/default.target") == "/usr/lib/systemd/system/graphical.target" ]]
grep -q 'pam_gnome_keyring.so' "$target/etc/pam.d/sddm"
test -f "$target/usr/share/icons/default/index.theme"
grep -q 'Inherits=Bibata-Modern-Ice' "$target/usr/share/icons/default/index.theme"

# 6. Verify lockscreen configuration
test -x "$target/home/ryoku/.local/share/quickshell-lockscreen/lock.sh"
test -f "$target/home/ryoku/.local/share/qylock/themes/clockwork/orbital/Main.qml"
[[ -L "$target/home/ryoku/.local/share/quickshell-lockscreen/themes_link" ]]
[[ $(readlink "$target/home/ryoku/.local/share/quickshell-lockscreen/themes_link") == "../qylock/themes" ]]
[[ $(cat "$target/home/ryoku/.config/qylock/theme") == "clockwork/orbital" ]]

# 7. Verify base system service unit links
[[ -L "$target/etc/systemd/system/multi-user.target.wants/NetworkManager.service" ]]
[[ -L "$target/etc/systemd/system/multi-user.target.wants/firewalld.service" ]]
[[ -L "$target/etc/systemd/system/bluetooth.target.wants/bluetooth.service" ]]

# 8. Verify assets and integrations
test -d "$target/home/ryoku/Pictures/Wallpapers"
test -d "$target/home/ryoku/Pictures/ryodecors"
test -d "$target/home/ryoku/.local/share/ryoku/assets/brand"
test -f "$target/usr/share/applications/ryoku-mimeapps.list"
test -f "$target/usr/share/applications/mimeapps.list"

# 9. Verify materialization and file ownership
test -s "$target/home/ryoku/.local/state/ryoku/materialized"
test -s "$target/home/ryoku/.config/hypr/settings.lua"
test -s "$target/home/ryoku/.config/hypr/rebinds.lua"
[[ $(stat -c '%u:%g' "$target/home/ryoku") == "1000:1000" ]]
unowned=$(find "$target/home/ryoku" -not -user 1000 -or -not -group 1000 | head -n 5)
if [[ -n $unowned ]]; then
  echo "Found files not owned by ryoku:ryoku: $unowned" >&2
  exit 1
fi

# 10. Verify firstboot armed state
test -f "$target/var/lib/ryoku-firstboot/armed"
test -x "$target/usr/libexec/ryoku-firstboot"
[[ -L "$target/etc/systemd/system/multi-user.target.wants/ryoku-firstboot.service" ]]
[[ -L "$target/etc/systemd/system/systemd-firstboot.service" ]]
[[ $(readlink "$target/etc/systemd/system/systemd-firstboot.service") == "/dev/null" ]]
test -f "$target/etc/systemd/system/sddm.service.d/10-firstboot.conf"

# 11. Verify systemd unit linkage
systemd-analyze --root="$target" verify ryoku-firstboot.service sddm.service

for binary in ryoku ryoku-wm-hyprland; do
  cp "$target/usr/bin/$binary" "$target/usr/bin/$binary.saved"
  printf '#!/bin/sh\necho intentional-test-failure >&2\nexit 7\n' > "$target/usr/bin/$binary"
  rm -f "$target/var/lib/ryoku-firstboot/armed"
  if python3 "$root/installation/fedora/provision-target.py" "$target" > "$target/failure.log" 2>&1; then
    echo "Provisioning incorrectly succeeded after $binary failed" >&2
    exit 1
  fi
  grep -q intentional-test-failure "$target/failure.log"
  test ! -e "$target/var/lib/ryoku-firstboot/armed"
  mv "$target/usr/bin/$binary.saved" "$target/usr/bin/$binary"
done

echo "Offline target provisioning validation passed."
