#!/usr/bin/env bash
# fixture test for ryoku_release_disk (installation/backend/lib/disk.sh): the
# pre-wipe teardown that frees a busy target disk so wipefs/sgdisk can't
# fail with "Device or resource busy". runs dry; lsblk is mocked and the
# /proc/mounts + /proc/swaps sources are redirected to fixtures via the
# RYOKU_PROC_MOUNTS / RYOKU_PROC_SWAPS seams (tests only), so what we assert is
# the teardown PLAN (commands, in order). no real device or /proc touched.
set -euo pipefail
export LC_ALL=C

here="$(cd "$(dirname "$0")" && pwd)"
root="$here/.."
fail() { echo "FAIL: $1" >&2; exit 1; }

# run ryoku_release_disk against a mocked block tree + injected /proc fixtures,
# capture the dry-run plan. RYOKU_PROC_MOUNTS / RYOKU_PROC_SWAPS are the (tests
# only) seams disk.sh reads instead of the real /proc, so the mounts + swapfiles
# are ours, not the host's; pvs is stubbed for the same reason (never query real
# LVM on the host's devices).
release() {
  RYOKU_DRYRUN=1 ROOT="$root" MOCK="$1" MOUNTS="${2:-}" SWAPS="${3:-}" bash -c '
    source "$ROOT/installation/backend/lib/common.sh"
    source "$ROOT/installation/backend/lib/disk.sh"
    [[ -n $MOUNTS ]] && export RYOKU_PROC_MOUNTS=$MOUNTS
    [[ -n $SWAPS ]] && export RYOKU_PROC_SWAPS=$SWAPS
    pvs() { :; }             # (tests only) never touch real LVM on host devices
    dmsetup() { return 1; }  # default: no stale mapper present; a case overrides.
    eval "$MOCK"
    ryoku_release_disk /dev/nvme0n1
  '
}

# --- busy disk: mounts (incl. a udisks auto-mount), a swapFILE, swap partition,
# and a LUKS holder. the /proc fixtures carry the mounts + swapfile; lsblk mocks
# the block tree (NAME for the tree, NAME,FSTYPE for swap parts, NAME,TYPE for dm).
tmp="$(mktemp -d)"; trap 'rm -rf "$tmp"' EXIT
cat >"$tmp/mounts" <<'EOF'
/dev/nvme0n1p2 /mnt btrfs rw,subvol=/@ 0 0
/dev/mapper/cr /run/media/u/DATA ext4 rw 0 0
EOF
cat >"$tmp/swaps" <<'EOF'
Filename Type Size Used Priority
/mnt/swap/swapfile file 1048576 0 -2
EOF
busy='lsblk() {
  case "$*" in
    *NAME,FSTYPE*) printf "/dev/nvme0n1p3 swap\n/dev/nvme0n1p1 vfat\n" ;;
    *NAME,TYPE*)   printf "/dev/nvme0n1 disk\n/dev/nvme0n1p2 part\n/dev/mapper/cr crypt\n" ;;
    *"NAME "*)     printf "/dev/nvme0n1\n/dev/nvme0n1p1\n/dev/nvme0n1p2\n/dev/nvme0n1p3\n/dev/mapper/cr\n" ;;
  esac
}'
out="$(release "$busy" "$tmp/mounts" "$tmp/swaps")"

grep -qF "umount -R -- '/mnt'" <<<"$out" || fail "did not unmount /mnt"
grep -qF "umount -R -- '/run/media/u/DATA'" <<<"$out" || fail "did not unmount the udisks auto-mount"
# every mountpoint of a partition comes back now (lsblk MOUNTPOINT showed one).
grep -qF "swapoff -- '/mnt/swap/swapfile'" <<<"$out" || fail "did not swapoff the installer swapFILE pinning the fs"
grep -qF "swapoff -- '/dev/nvme0n1p3'" <<<"$out" || fail "did not swapoff the swap partition"
grep -qF "swapoff -- '/dev/nvme0n1p1'" <<<"$out" && fail "swapoff hit the non-swap vfat partition"
grep -qF "cryptsetup close -- '/dev/mapper/cr'" <<<"$out" || fail "did not close the LUKS/dm holder"
grep -qF "udevadm settle" <<<"$out" || fail "did not settle udev"

# deepest mount released before its parent (udisks before /mnt).
first_umount="$(grep 'umount -R' <<<"$out" | head -n1)"
[[ $first_umount == *"/run/media/u/DATA"* ]] || fail "unmount order is not deepest-first (got: $first_umount)"

# the swapFILE is freed BEFORE /mnt is unmounted (a live swapfile pins the fs).
sf_line="$(grep -nF "swapoff -- '/mnt/swap/swapfile'" <<<"$out" | head -n1 | cut -d: -f1)"
mnt_line="$(grep -nF "umount -R -- '/mnt'" <<<"$out" | head -n1 | cut -d: -f1)"
(( sf_line < mnt_line )) || fail "swapfile swapoff must precede the /mnt unmount"

# --- clean disk: no holders -> a quiet no-op (no destructive actions) ---------
out="$(release 'lsblk() { printf "\n"; }')"
grep -qF 'releasing /dev/nvme0n1' <<<"$out" || fail "missing the releasing log line"
grep -qE "umount -R|swapoff --|cryptsetup close --|dmsetup remove|vgchange -an|mdadm --stop" <<<"$out" \
  && fail "acted on a clean disk that has no holders"

# --- orphaned /dev/mapper/root from a failed prior run: freed by NAME ---------
# lsblk ties nothing to the disk (the orphan's backing partition is already
# gone), so only the by-name free reaches it; dmsetup reports the name present.
out="$(release 'dmsetup() { return 0; }
lsblk() { printf "\n"; }')"
grep -qF "freeing stale mapper /dev/mapper/root" <<<"$out" || fail "did not announce freeing the orphaned root mapper"
grep -qF "cryptsetup close -- 'root'" <<<"$out" || fail "did not close the orphaned root mapper by name"
grep -qF "swapoff -a" <<<"$out" || fail "did not swapoff -a to release a swapfile that could pin the mapper"

echo "install-disk-teardown: all checks passed"
