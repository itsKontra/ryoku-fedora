#!/usr/bin/env bash
#
# Builds the Ryoku Fedora ISO: the Fedora Everything netinst image remastered
# with mkksiso so Anaconda installs a minimal Fedora and post/provision turns
# it into Ryoku from the signed RPM repository at repo.ryoku.dev. Nothing is
# bundled offline; the install downloads every package.
#
#   build.sh --base-iso <netinst.iso> [options]
#
#   --release <N>          Fedora release (default: the first in fedora/releases)
#   --channel <name>       stable, testing, or a release tag like v0.101.0
#                          (default: stable)
#   --compositor <id>      default desktop (default: choices.tsv's policy)
#   --browser <id>         default browser (default: choices.tsv's policy)
#   --shell <id>           default login shell (default: choices.tsv's policy)
#   --out <dir>            output directory (default: fedora/iso/out)
#   --work <dir>           scratch directory (default: fedora/iso/work)
#   --iso-name <file>      output file name
#   --skip-repo-check      do not require the channel's repository to answer
#   --dev                  build as a normal user without updating the EFI
#                          boot image; such an ISO may not boot from UEFI USB
#                          and must not be published
#
# The ryoku.compositor=, ryoku.browser= and ryoku.shell= boot options override
# the defaults at install time; ryoku.nvidia=0 keeps a supported NVIDIA GPU on
# nouveau instead of installing the NVIDIA driver.
set -euo pipefail

here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
repo=$(cd "$here/../.." && pwd)

base_iso='' release='' channel=stable compositor='' browser='' login_shell=''
out=$here/out work=$here/work iso_name='' repo_check=1 dev=0
while (($#)); do
  case $1 in
    --base-iso) base_iso=$2; shift 2 ;;
    --release) release=$2; shift 2 ;;
    --channel) channel=$2; shift 2 ;;
    --compositor) compositor=$2; shift 2 ;;
    --browser) browser=$2; shift 2 ;;
    --shell) login_shell=$2; shift 2 ;;
    --out) out=$2; shift 2 ;;
    --work) work=$2; shift 2 ;;
    --iso-name) iso_name=$2; shift 2 ;;
    --skip-repo-check) repo_check=0; shift ;;
    --dev) dev=1; shift ;;
    -h | --help) sed -n '3,/^set /{/^#/p}' "$0" | cut -c3-; exit 0 ;;
    *) echo "build.sh: unknown option: $1" >&2; exit 2 ;;
  esac
done

say() { printf ':: %s\n' "$*"; }
die() { printf 'build.sh: %s\n' "$*" >&2; exit 1; }

[[ -f $base_iso ]] || die "--base-iso must name the Fedora netinst ISO (fetch-base.sh downloads it)"
[[ -n $release ]] || release=$(grep -m1 -E '^[0-9]+$' "$repo/fedora/releases")
grep -qx "$release" "$repo/fedora/releases" || die "Fedora $release is not in fedora/releases"
for tool in mkksiso xorriso cpio gzip curl; do
  command -v "$tool" >/dev/null || die "$tool is required"
done
((dev || EUID == 0)) || die "mkksiso needs root to update the EFI boot image (or pass --dev)"

# The repository layout matches fedoraRepository() in the shell installer.
case $channel in
  stable) path=x86_64 ;;
  testing) path=channels/testing/x86_64 ;;
  v[0-9]*.[0-9]*.[0-9]*) path=releases/$channel/x86_64 ;;
  *) die "unknown channel: $channel" ;;
esac
repo_url="https://repo.ryoku.dev/stable/fedora/\$releasever/$path"
if ((repo_check)); then
  probe=${repo_url//\$releasever/$release}/repodata/repomd.xml
  curl -fsSI "$probe" >/dev/null || die "the $channel repository for Fedora $release does not answer: $probe"
fi

commit=$(git -C "$repo" rev-parse --short HEAD)
volid="Ryoku-$release-x86_64"
[[ -n $iso_name ]] || iso_name="ryoku-fedora-$release-$channel-$commit-x86_64.iso"

say "staging the installer payload"
stage=$work/ryoku-iso
rm -rf "$stage"
mkdir -p "$stage/post" "$out"
install -m755 "$here/post/stage" "$here/post/provision" "$here/post/check-admin" "$stage/post/"
for path in \
  system/packages/resolve \
  system/hardware/drivers/nvidia.sh \
  fedora/packages/translations.tsv \
  fedora/packages/coprs.tsv \
  installation/installerplan/choices.tsv \
  ryoku/lockscreen/sddm/setup \
  ryoku/assets/brand \
  ryoku/assets/wallpapers \
  ryoku/apps/npm/npmrc \
  ryoku/apps/nvim/ryoku-nvim.desktop; do
  mkdir -p "$stage/payload/$(dirname "$path")"
  cp -a "$repo/$path" "$stage/payload/$path"
done
{
  printf 'RYOKU_ISO_CHANNEL=%q\n' "$channel"
  printf 'RYOKU_ISO_REPO=%q\n' "$repo_url"
  printf 'RYOKU_ISO_KEY_URL=%q\n' https://repo.ryoku.dev/stable/ryoku-release-key.pub.asc
  printf 'RYOKU_ISO_KEY_FINGERPRINT=%q\n' EB6D3C0F55A7B3CABA6B2838847B274F025DD6E3
  printf 'RYOKU_ISO_COMPOSITOR=%q\n' "$compositor"
  printf 'RYOKU_ISO_BROWSER=%q\n' "$browser"
  printf 'RYOKU_ISO_SHELL=%q\n' "$login_shell"
  printf 'RYOKU_ISO_COMMIT=%q\n' "$commit"
} >"$stage/ryoku-iso.conf"

say "rendering the kickstart"
kickstart=$work/ryoku.ks
sed -e "s/@RELEASE@/$release/g" -e "s/@VOLID@/$volid/g" "$here/kickstart/ryoku.ks" >"$kickstart"
if command -v ksvalidator >/dev/null; then
  ksvalidator -v "F$release" "$kickstart" 2>/dev/null || ksvalidator -v DEVEL "$kickstart"
fi

say "packing the Anaconda drop-in into an updates image"
updates=$work/updates.img
rm -rf "$work/updates"
install -Dm644 "$here/anaconda/05-ryoku.conf" "$work/updates/etc/anaconda/conf.d/05-ryoku.conf"
(cd "$work/updates" && find . -mindepth 1 | LC_ALL=C sort | cpio --quiet -o -H newc | gzip -9n) >"$updates"

# boot_layout <iso>: fail unless the image keeps a UEFI El Torito entry and the
# hybrid GPT that lets it boot from a USB stick.
boot_layout() {
  local report
  report=$(xorriso -indev "$1" -report_el_torito plain -report_system_area plain 2>&1) ||
    die "cannot read the boot layout of $1"
  grep -Eq 'El Torito boot img :.*UEFI' <<<"$report" || die "$1 has no UEFI El Torito boot entry"
  grep -Eq 'System area summary:.*GPT' <<<"$report" || die "$1 has no hybrid GPT boot layout"
}

boot_layout "$base_iso"
iso=$out/$iso_name
rm -f "$iso" "$iso.sha256"
args=(--ks "$kickstart" -u "$updates" -a "$stage" -V "$volid"
  -R 'Install Fedora' 'Install Ryoku on Fedora' -R 'install Fedora' 'install Ryoku on Fedora')
((dev == 0)) || args+=(--skip-mkefiboot)
say "remastering $(basename "$base_iso") into $iso_name"
mkksiso "${args[@]}" "$base_iso" "$iso"
boot_layout "$iso"
(cd "$out" && sha256sum "$iso_name" >"$iso_name.sha256")
say "built $iso"
