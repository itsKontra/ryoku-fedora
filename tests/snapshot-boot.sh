#!/usr/bin/env bash
# hermetic test for system/bootmenu, the Ryoku GRUB theme and the snapshots
# bootable from the GRUB menu (issue #48).
#
# case 1: the payload, the spec, the units and the CLI agree on names and
#         paths, and the snapper plugin resyncs only after a snapshot changed.
# case 2: the initramfs restore hook, driven with stubbed dracut helpers
#         against a fake btrfs top level: it swaps the snapshot in and keeps
#         the old root, and leaves the disk alone on anything unexpected.
# case 3: the initramfs look hook stacks a RAM overlay on the snapshot.
# case 4: ryoku-grub-menu themes GRUB idempotently and purge undoes it.
# shellcheck disable=SC2329 # stubs and predicates run indirectly
# shellcheck disable=SC2016 # single-quoted patterns are literal file text

set -euo pipefail

repo="$(cd "$(dirname "$0")/.." && pwd -P)"
bm="$repo/system/bootmenu"
mod="$bm/dracut/90ryoku-snapshot"
payload="$repo/release/rpm/payload/ryoku-desktop.sh"
spec="$repo/release/rpm/ryoku-desktop.spec"
cli="$repo/ryoku/cli/internal/updater/bootmenu.go"

work=$(mktemp -d /tmp/ryoku-snapshot-boot.XXXXXX)
trap 'rm -rf "$work"' EXIT

fail=0
ok() { printf 'ok   %s\n' "$1"; }
bad() { printf 'FAIL %s\n' "$1"; fail=1; }
check() { if "${@:2}"; then ok "$1"; else bad "$1"; fi; }
line() { grep -qxF -- "$2" "$1"; }
has() { grep -qF -- "$2" "$1"; }
absent() { [ ! -e "$1" ]; }

# case 1
for dest in \
  usr/bin/ryoku-grub-menu \
  etc/grub.d/42_ryoku_snapshots \
  usr/share/ryoku/grub/theme/theme.txt \
  usr/libexec/snapper/plugins/50-ryoku-boot-menu \
  usr/lib/systemd/system/ryoku-snapshot-menu.service \
  usr/lib/systemd/system/ryoku-snapshot-restored.service; do
  check "ryoku-desktop ships /$dest" has "$payload" "\"\$pkgdir/$dest\""
done
check "ryoku-desktop ships the dracut module" has "$payload" '"$pkgdir/usr/lib/dracut/modules.d/90ryoku-snapshot/$mod"'
check "ryoku ships the snapper config the installer seeds" has "$repo/release/rpm/payload/ryoku.sh" \
  '"$pkgdir/usr/share/ryoku/snapper/root.conf"'
check "the doctor embeds that same file" has "$repo/ryoku/cli/internal/doctor/doctor.go" "//go:embed snapper-root.conf"
check "%posttrans themes GRUB" has "$spec" "/usr/bin/ryoku-grub-menu install"
check "%posttrans lists the snapshots on disk" has "$spec" "systemctl start --no-block ryoku-snapshot-menu.service"
check "%post enables the restore follow-up" has "$spec" "systemctl enable ryoku-snapshot-restored.service"
check "%preun undoes the theme and the menu" has "$spec" "/usr/bin/ryoku-grub-menu purge"
check "%postun drops them from grub.cfg" has "$spec" "grub2-mkconfig -o /boot/grub2/grub.cfg"
for dep in grub2-tools btrfs-progs snapper dracut; do
  check "ryoku-desktop requires $dep" grep -qE "^Requires: +$dep\$" "$spec"
done
check "the menu unit runs the CLI sync" line "$bm/ryoku-snapshot-menu.service" "ExecStart=/usr/bin/ryoku boot-menu sync"
check "the restored unit runs the CLI follow-up" line "$bm/ryoku-snapshot-restored.service" "ExecStart=/usr/bin/ryoku boot-menu restored"
check "the restored unit waits for the hook's flag" line "$bm/ryoku-snapshot-restored.service" "ConditionPathExists=/run/ryoku/restored"
check "the CLI reads the flag the hook writes" grep -q 'restoredFlag *= "/run/ryoku/restored"' "$cli"
check "the CLI builds images with the shipped module" grep -q 'snapshotDracutMod *= "ryoku-snapshot"' "$cli"
check "the CLI names the snippet the package ships" grep -q 'snapshotMenuScript *= "/etc/grub.d/42_ryoku_snapshots"' "$cli"
check "the snippet sources the file the CLI writes" has "$bm/grub/42_ryoku_snapshots" 'source "${config_directory}/ryoku-snapshots.cfg"'
check "the grub.d snippet renders" sh -c "sh '$bm/grub/42_ryoku_snapshots' | grep -q '^if \\[ -f '"
check "the module is only built in on request" grep -q '^  return 255$' "$mod/module-setup.sh"

stubs="$work/bin"
mkdir -p "$stubs"
printf '#!/bin/sh\necho "$*" >>"%s/systemctl.log"\n' "$work" >"$stubs/systemctl"
chmod +x "$stubs/systemctl"
for event in create-snapshot-pre create-snapshot-post modify-snapshot-post delete-snapshot-post; do
  PATH="$stubs:$PATH" sh "$bm/snapper/50-ryoku-boot-menu" "$event" / btrfs 42
done
check "the plugin resyncs after a snapshot is created or deleted, only" test \
  "$(cat "$work/systemctl.log")" = "start --no-block ryoku-snapshot-menu.service
start --no-block ryoku-snapshot-menu.service"

# case 2
# hook <script> <cmdline>: source a dracut hook with the dracut-lib helpers
# stubbed, the root device at /dev/null and /run at $work/run.
hook() {
  local script="$1" cmdline="$2"
  (
    set +eu
    export RYOKU_RUN="$work/run" NEWROOT="$work/sysroot"
    getarg() {
      local tok
      for tok in $cmdline; do
        case "$tok" in "$1"*) echo "${tok#"$1"}"; return 0 ;; esac
      done
      return 1
    }
    warn() { echo "warn: $*" >>"$work/hook.log"; }
    info() { echo "info: $*" >>"$work/hook.log"; }
    label_uuid_to_dev() { echo /dev/null; }
    mount() { echo "mount $*" >>"$work/mount.log"; [ -z "${MOUNT_FAILS:-}" ]; }
    umount() { :; }
    date() { echo 20260926-120000; }
    btrfs() {
      [ -z "${SNAPSHOT_FAILS:-}" ] || return 1
      [ "$1 $2" = "subvolume snapshot" ] && cp -a "$3" "$4"
    }
    # shellcheck source=/dev/null
    . "$script"
  )
}

top="$work/run/ryoku/restore-top"
fresh_disk() {
  rm -rf "$work/run" "$work/hook.log" "$work/mount.log"
  mkdir -p "$top/root" "$top/snapshots/42/snapshot"
  echo current >"$top/root/state"
  echo before >"$top/snapshots/42/snapshot/state"
}
restore_args="root=UUID=abc ro rootflags=subvol=root,compress=zstd:1 rd.ryoku.restore=snapshots/42/snapshot"
state() { cat "$top/$1/state" 2>/dev/null; }

fresh_disk
hook "$mod/ryoku-snapshot-restore.sh" "$restore_args"
check "restore mounts the btrfs top level" grep -q "^mount -t btrfs -o subvolid=5 /dev/null $top\$" "$work/mount.log"
check "restore puts the snapshot in place of the root" test "$(state root)" = before
check "restore keeps the replaced root" test "$(state root.broken-20260926-120000)" = current
check "restore leaves the snapshot itself alone" test "$(state snapshots/42/snapshot)" = before
check "restore tells the booted system what happened" line "$work/run/ryoku/restored" \
  "snapshots/42/snapshot root.broken-20260926-120000"

fresh_disk
hook "$mod/ryoku-snapshot-restore.sh" "root=UUID=abc rootflags=subvol=root"
check "no restore argument, nothing happens" test "$(state root)" = current -a ! -e "$work/mount.log"

for arg in "../root/snapshot" "/snapshots/42/snapshot" "snapshots/42" "snap shots/42/snapshot"; do
  fresh_disk
  hook "$mod/ryoku-snapshot-restore.sh" "root=UUID=abc rootflags=subvol=root rd.ryoku.restore=$arg"
  check "restore refuses '$arg'" test "$(state root)" = current -a ! -e "$work/run/ryoku/restored"
done

fresh_disk
hook "$mod/ryoku-snapshot-restore.sh" "root=UUID=abc rootflags=compress=zstd:1 rd.ryoku.restore=snapshots/42/snapshot"
check "restore needs the root subvolume from rootflags" test "$(state root)" = current -a ! -e "$work/mount.log"

fresh_disk
hook "$mod/ryoku-snapshot-restore.sh" "root=UUID=abc rootflags=subvol=root rd.ryoku.restore=snapshots/7/snapshot"
check "a missing snapshot leaves the root alone" test "$(state root)" = current -a ! -e "$work/run/ryoku/restored"

fresh_disk
SNAPSHOT_FAILS=1 hook "$mod/ryoku-snapshot-restore.sh" "$restore_args"
check "the hook got as far as the snapshot" grep -q "snapshotting snapshots/42/snapshot failed" "$work/hook.log"
check "a failed snapshot puts the root back" test "$(state root)" = current -a ! -e "$top/root.broken-20260926-120000"
check "a failed restore is not reported as done" absent "$work/run/ryoku/restored"

# case 3
rm -rf "$work/run" "$work/mount.log"
hook "$mod/ryoku-snapshot-preview.sh" "root=UUID=abc ro rootflags=subvol=snapshots/42/snapshot rd.ryoku.preview=snapshots/42/snapshot"
base="$work/run/ryoku/preview"
check "look mounts the snapshot read-only as the lower layer" \
  grep -qxF "mount -t btrfs -o ro,subvol=snapshots/42/snapshot /dev/null $base/lower" "$work/mount.log"
check "look keeps its writes in RAM" grep -qxF "mount -t tmpfs -o mode=0755 ryoku-preview $base/rw" "$work/mount.log"
check "look stacks the overlay on the root" grep -qxF \
  "mount -t overlay ryoku-preview -o lowerdir=$base/lower,upperdir=$base/rw/upper,workdir=$base/rw/work $work/sysroot" \
  "$work/mount.log"

rm -f "$work/mount.log"
hook "$mod/ryoku-snapshot-preview.sh" "root=UUID=abc ro rootflags=subvol=root"
check "no look argument, nothing is mounted" absent "$work/mount.log"
hook "$mod/ryoku-snapshot-preview.sh" "root=UUID=abc rd.ryoku.preview=../x"
check "look refuses a path outside the filesystem" absent "$work/mount.log"

# case 4
grub="$work/boot/grub2"
mkdir -p "$grub" "$work/etc"
printf 'GRUB_TIMEOUT=1\nGRUB_DEFAULT=saved\nGRUB_TERMINAL_OUTPUT="console"\nGRUB_ENABLE_BLSCFG=true\n' >"$work/etc/grub"
printf 'PFF2' >"$work/unicode.pf2"
cat >"$stubs/mkconfig" <<EOF
#!/bin/sh
echo run >>"$work/mkconfig.log"
sh "$bm/grub/42_ryoku_snapshots" >"\$2"
EOF
chmod +x "$stubs/mkconfig"
menu() {
  RYOKU_GRUB_DIR="$grub" RYOKU_GRUB_DEFAULTS="$work/etc/grub" RYOKU_GRUB_THEME_SRC="$bm/grub/theme" \
    RYOKU_GRUB_FONT_SRC="$work/unicode.pf2" RYOKU_GRUB_MKCONFIG="$stubs/mkconfig" bash "$bm/grub/ryoku-grub-menu" "$@"
}
runs() { test "$(wc -l <"$work/mkconfig.log" 2>/dev/null || echo 0)" = "$1"; }

menu install
check "install copies the theme onto /boot" test -f "$grub/themes/ryoku/theme.txt" -a -f "$grub/themes/ryoku/unicode.pf2"
check "install turns on the graphical terminal" line "$work/etc/grub" 'GRUB_TERMINAL_OUTPUT="gfxterm"'
check "install points GRUB at the theme" line "$work/etc/grub" "GRUB_THEME=\"$grub/themes/ryoku/theme.txt\""
check "install takes the font from /boot, not the root filesystem" line "$work/etc/grub" \
  "GRUB_FONT=\"$grub/themes/ryoku/unicode.pf2\""
check "install keeps the rest of the defaults" line "$work/etc/grub" "GRUB_ENABLE_BLSCFG=true"
check "install rebuilds grub.cfg with the snapshot menu" has "$grub/grub.cfg" "ryoku-snapshots.cfg"
menu install
check "a second install changes nothing and does not rebuild" runs 1
check "a second install does not repeat a key" test "$(grep -c '^GRUB_THEME=' "$work/etc/grub")" = 1
: >"$grub/grub.cfg"
menu install
check "install rebuilds a grub.cfg that lost the snapshot menu" runs 2

mkdir -p "$work/boot/ryoku/snapshots/k" && : >"$grub/ryoku-snapshots.cfg"
menu purge
check "purge restores the text terminal" line "$work/etc/grub" 'GRUB_TERMINAL_OUTPUT="console"'
check "purge drops the theme keys" test -z "$(grep -E '^GRUB_(THEME|FONT)=' "$work/etc/grub")"
check "purge removes the theme, the menu and the kernel copies" \
  test ! -e "$grub/themes/ryoku" -a ! -e "$grub/ryoku-snapshots.cfg" -a ! -e "$work/boot/ryoku"

printf 'GRUB_TERMINAL_OUTPUT="serial console"\n' >"$work/etc/grub"
menu install
check "install keeps a serial terminal" line "$work/etc/grub" 'GRUB_TERMINAL_OUTPUT="serial console"'
check "install still themes the graphical menu there" grep -q '^GRUB_THEME=' "$work/etc/grub"
menu purge
check "purge leaves a serial terminal alone" line "$work/etc/grub" 'GRUB_TERMINAL_OUTPUT="serial console"'

exit "$fail"
