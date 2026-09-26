#!/usr/bin/env bash
# hermetic test for system/recovery, the console fallback when the desktop
# cannot start (issue #49).
#
# case 1: the sddm drop-in, the fallback and guard units, and the payload agree
#         on names and paths, so OnFailure and the guard's hold-back actually
#         reach the units the package ships.
# case 2: the kernel-install plugin twins a Fedora BLS entry into a "Ryoku
#         console" entry that boots multi-user.target, leaves rescue and debug
#         entries alone, and sync/remove/purge keep the twins in step.
# shellcheck disable=SC2329 # the predicates run through check, indirectly

set -euo pipefail

repo="$(cd "$(dirname "$0")/.." && pwd -P)"
rec="$repo/system/recovery"
payload="$repo/release/rpm/payload/ryoku-desktop.sh"
spec="$repo/release/rpm/ryoku-desktop.spec"
plugin="$rec/95-ryoku-console.install"

work=$(mktemp -d /tmp/ryoku-console-recovery.XXXXXX)
trap 'rm -rf "$work"' EXIT

fail=0
ok() { printf 'ok   %s\n' "$1"; }
bad() { printf 'FAIL %s\n' "$1"; fail=1; }
check() { if "${@:2}"; then ok "$1"; else bad "$1"; fi; }
line() { grep -qxF -- "$2" "$1"; }

# case 1
dropin="$rec/sddm-console-fallback.conf"
fallback="$rec/ryoku-console-fallback.service"
guard="$rec/ryoku-console-guard.service"
cli="$repo/ryoku/cli/internal/updater/consoleguard.go"

check "sddm failure starts the console fallback" line "$dropin" "OnFailure=ryoku-console-fallback.service"
check "the guard's flag holds sddm back" line "$dropin" "ConditionPathExists=!/run/ryoku/console-boot"
check "sddm pulls the guard in" line "$dropin" "Wants=ryoku-console-guard.service"
check "the guard decides before sddm starts" line "$dropin" "After=ryoku-console-guard.service"
check "the fallback brings up a login on tty1" line "$fallback" "Wants=getty@tty1.service"
check "the banner is in place before agetty prints it" line "$fallback" "Before=getty@tty1.service"
check "the fallback installs the shipped banner" grep -q \
  ' /usr/share/ryoku/recovery/console-fallback.issue /etc/issue.d/ryoku-console-fallback.issue$' "$fallback"
check "the banner is cleared on the next boot" line "$rec/ryoku-recovery.tmpfiles.conf" \
  "r! /etc/issue.d/ryoku-console-fallback.issue"
check "the guard runs the CLI's console check" line "$guard" "ExecStart=/usr/bin/ryoku boot-guard --console"
check "the CLI writes the flag the drop-in checks" grep -q 'consoleBootFlag *= "/run/ryoku/console-boot"' "$cli"
check "the CLI starts the shipped fallback unit" grep -q '"ryoku-console-fallback.service"' "$cli"
check "no passwordless emergency shell" \
  bash -c '! grep -rq SYSTEMD_SULOGIN_FORCE "$@"' _ "$rec" "$repo/installation" "$repo/release"

for dest in \
  usr/lib/systemd/system/sddm.service.d/50-ryoku-console-fallback.conf \
  usr/lib/systemd/system/ryoku-console-fallback.service \
  usr/lib/systemd/system/ryoku-console-guard.service \
  usr/share/ryoku/recovery/console-fallback.issue \
  usr/lib/tmpfiles.d/ryoku-recovery.conf \
  usr/lib/kernel/install.d/95-ryoku-console.install; do
  check "ryoku-desktop ships /$dest" grep -qF "\"\$pkgdir/$dest\"" "$payload"
done
check "%post twins the installed kernels" grep -q '95-ryoku-console.install sync' "$spec"
check "%preun drops the twins on removal" grep -q '95-ryoku-console.install purge' "$spec"

# the guard's ExecStart is the packaged CLI, which a CI runner lacks; the
# fallback only names stock binaries, so it verifies anywhere.
if command -v systemd-analyze >/dev/null 2>&1; then
  check "systemd-analyze accepts the fallback unit" systemd-analyze verify --man=no "$fallback"
fi

# case 2
bls="$work/entries"
mkdir -p "$bls"
mid=0123456789abcdef0123456789abcdef
kver=6.17.1-300.fc44.x86_64
old=6.16.9-200.fc44.x86_64
gone=6.1.0-1.fc44.x86_64
cat >"$bls/$mid-$kver.conf" <<EOF
title Fedora Linux ($kver) 44 (Forty Four)
version $kver
linux /vmlinuz-$kver
initrd /initramfs-$kver.img
options root=UUID=abc ro rootflags=subvol=root rhgb quiet
grub_users \$grub_users
grub_arg --unrestricted
grub_class fedora
EOF
printf 'title rescue\nversion 0-rescue\nlinux /vmlinuz-0-rescue\noptions root=UUID=abc ro\n' >"$bls/$mid-0-rescue-$mid.conf"
printf 'title debug\nversion %s\noptions root=UUID=abc ro\n' "$kver" >"$bls/$mid-$kver~debug.conf"

run() { RYOKU_BLS_DIR="$bls" KERNEL_INSTALL_MACHINE_ID="$mid" bash "$plugin" "$@"; }
twin="$bls/$mid-$kver~ryoku-console.conf"
absent() { [ ! -e "$1" ]; }
no_side_twins() { absent "$bls/$mid-0-rescue-$mid~ryoku-console.conf" && absent "$bls/$mid-$kver~debug~ryoku-console.conf"; }

run add "$kver" "$work/unused" "/usr/lib/modules/$kver/vmlinuz"
check "add twins the kernel's entry" test -f "$twin"
check "the twin is labelled Ryoku console" line "$twin" "title Ryoku console: Fedora Linux ($kver) 44 (Forty Four)"
check "the twin sorts just below its kernel" line "$twin" "version $kver~ryoku-console"
check "the twin boots to a text login" line "$twin" \
  "options root=UUID=abc ro rootflags=subvol=root rhgb quiet systemd.unit=multi-user.target"
check "the twin boots the same kernel" line "$twin" "linux /vmlinuz-$kver"
check "the twin keeps the GRUB arguments" line "$twin" "grub_arg --unrestricted"
check "add leaves rescue and debug entries alone" no_side_twins

run add "$kver"
check "add is idempotent" test "$(grep -c 'systemd.unit=multi-user.target' "$twin")" = 1

run remove "$kver"
check "remove drops the twin" absent "$twin"

sed "s/$kver/$old/g" "$bls/$mid-$kver.conf" >"$bls/$mid-$old.conf"
printf 'title gone\noptions x\n' >"$bls/$mid-$gone~ryoku-console.conf"
run sync
check "sync twins every kernel" test -f "$twin" -a -f "$bls/$mid-$old~ryoku-console.conf"
check "sync drops orphaned twins" absent "$bls/$mid-$gone~ryoku-console.conf"
check "sync leaves rescue and debug entries alone" no_side_twins

printf 'title no options\nversion 9.9\nlinux /vmlinuz-9.9\n' >"$bls/$mid-9.9.conf"
run add 9.9
check "an entry without options is not duplicated" absent "$bls/$mid-9.9~ryoku-console.conf"

run purge
check "purge drops every twin" test -z "$(find "$bls" -name '*~ryoku-console.conf')"
check "purge keeps Fedora's entries" test -f "$bls/$mid-$kver.conf" -a -f "$bls/$mid-$old.conf"

exit "$fail"
