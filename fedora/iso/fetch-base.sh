#!/usr/bin/env bash
#
# Downloads the Fedora Everything netinst ISO the Ryoku ISO is built from and
# checks it against Fedora's CHECKSUM file. A released Fedora is read from
# releases/<N> through the mirrors, and its CHECKSUM must carry the release
# key's signature. A Fedora still in development is read from development/<N>
# on Fedora's own server: nightly composes are not signed, so only the HTTPS
# checksum vouches for that image. A file already in the destination that
# matches the checksum is kept.
#
#   fetch-base.sh --release <N> --dest <dir>   prints the ISO's path
set -euo pipefail

release=
dest=
while (($#)); do
  case $1 in
    --release) release=$2; shift 2 ;;
    --dest) dest=$2; shift 2 ;;
    *) echo "fetch-base.sh: unknown option: $1" >&2; exit 2 ;;
  esac
done
[[ $release =~ ^[0-9]+$ && -n $dest ]] || { echo "usage: fetch-base.sh --release <N> --dest <dir>" >&2; exit 2; }

say() { printf ':: %s\n' "$*" >&2; }
die() { printf 'fetch-base.sh: %s\n' "$*" >&2; exit 1; }

index=https://dl.fedoraproject.org/pub/fedora/linux
mirror=https://download.fedoraproject.org/pub/fedora/linux
listing='' tree=''
for tree in releases development; do
  listing=$(curl -fsSL "$index/$tree/$release/Everything/x86_64/iso/" 2>/dev/null) && break
  listing=
done
[[ -n $listing ]] || die "Fedora $release has no Everything ISO directory under releases/ or development/"

iso=$(grep -o "Fedora-Everything-netinst-x86_64-$release-[0-9A-Za-z.]*\.iso" <<<"$listing" | sort -uV | tail -n1)
checksum=$(grep -oE "Fedora-Everything-[0-9A-Za-z_.-]*${release}[0-9A-Za-z_.-]*-CHECKSUM" <<<"$listing" | sort -uV | tail -n1)
[[ -n $iso && -n $checksum ]] || die "no netinst ISO and CHECKSUM listed for Fedora $release ($tree)"
base=$mirror/$tree/$release/Everything/x86_64/iso
[[ $tree == releases ]] || base=$index/$tree/$release/Everything/x86_64/iso

mkdir -p "$dest"
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
curl -fsSL --retry 3 -o "$work/CHECKSUM" "$base/$checksum"

keyring=$work/fedora.gpg
local_key=/etc/pki/rpm-gpg/RPM-GPG-KEY-fedora-$release-primary
if [[ -f $local_key ]]; then
  cp "$local_key" "$keyring.asc"
else
  say "no local Fedora $release key; using https://fedoraproject.org/fedora.gpg"
  curl -fsSL --retry 3 -o "$keyring.asc" https://fedoraproject.org/fedora.gpg
fi
gpg --batch --quiet --yes --dearmor -o "$keyring" "$keyring.asc"
if ! gpgv --keyring "$keyring" "$work/CHECKSUM" 2>/dev/null; then
  [[ $tree == development ]] || die "the CHECKSUM signature does not verify against the Fedora $release key"
  say "warning: Fedora $release is a development compose with an unsigned CHECKSUM"
fi

expected=$(sed -n "s/^SHA256 ($iso) = \([0-9a-f]\{64\}\)$/\1/p" "$work/CHECKSUM")
[[ -n $expected ]] || die "$checksum lists no SHA256 for $iso"

target=$dest/$iso
matches() { [[ -f $target ]] && [[ $(sha256sum "$target" | cut -d' ' -f1) == "$expected" ]]; }
if matches; then
  say "reusing $target"
else
  say "downloading $iso ($tree)"
  curl -fsSL --retry 3 --retry-delay 5 -o "$target.part" "$base/$iso"
  mv -f "$target.part" "$target"
  matches || die "$iso does not match its signed checksum"
fi
printf '%s\n' "$target"
