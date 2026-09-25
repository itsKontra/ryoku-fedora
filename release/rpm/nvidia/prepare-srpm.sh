#!/usr/bin/env bash
# Build NVIDIA's open modules from RPM Fusion's source for the newest Fedora
# kernels, sign them with the Ryoku module key, and wrap the signed modules in
# the ryoku-nvidia SRPM. COPR then only packages them: it never holds the key.
#
# Runs as root in a Fedora container with RPM Fusion nonfree enabled.
#   RYOKU_NVIDIA_OUT          new directory for the SRPM and release.json
#   RYOKU_NVIDIA_MOK_KEY_FILE private key matching ryoku-mok.der
#   RYOKU_RPM_REVISION        RPM Release, monotonic across builds
#   RYOKU_NVIDIA_PUBLISHED    repository URL; exit early when it is current
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$here/../../.." && pwd)
die() { echo "prepare-srpm: $*" >&2; exit 1; }

out=$(realpath -m "${RYOKU_NVIDIA_OUT:?}")
key=${RYOKU_NVIDIA_MOK_KEY_FILE:?}
cert=$here/ryoku-mok.der
revision=${RYOKU_RPM_REVISION:-1}
keys=$root/installation/fedora/keys

[[ $revision =~ ^[1-9][0-9]*$ ]] || die 'RPM revision must be a positive integer'
[[ ! -e $out ]] || die "output already exists: $out"
[[ -f $cert ]] || die "missing $cert; run generate-mok-key.sh first"
[[ $(openssl pkey -in "$key" -pubout) == "$(openssl x509 -inform DER -in "$cert" -noout -pubkey)" ]] ||
  die 'the private key does not belong to ryoku-mok.der'

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

rpmkeys --import "$keys/RPM-GPG-KEY-fedora-44-primary" "$keys/RPM-GPG-KEY-rpmfusion-nonfree-fedora-2020"
fedora_key=$(gpg --show-keys --with-colons "$keys/RPM-GPG-KEY-fedora-44-primary" |
  awk -F: '$1 == "fpr" { print substr($10, 33); exit }')
mapfile -t kernels < <(python3 "$here/fetch-kernels.py" "$work/kernels" --key-id "$fedora_key")
(( ${#kernels[@]} )) || die 'no Fedora kernels found'

driver=$(dnf repoquery -q --latest-limit=1 --qf '%{version}\n' xorg-x11-drv-nvidia)
[[ $driver =~ ^[0-9]+(\.[0-9]+)+$ ]] || die "unexpected RPM Fusion driver version: $driver"
echo "driver $driver for kernels ${kernels[*]}"

if [[ -n ${RYOKU_NVIDIA_PUBLISHED:-} ]]; then
  published=$(dnf repoquery -q --repofrompath="published,$RYOKU_NVIDIA_PUBLISHED" --repo=published \
    --qf '%{version}\n' "ryoku-nvidia-kmod-${kernels[0]}" 2>/dev/null || true)
  if [[ $published == "$driver" ]]; then
    echo "ryoku-nvidia $driver for ${kernels[0]} is already published"
    exit 0
  fi
fi

dnf -y install "$work"/kernels/*.rpm
dnf -y install akmods kmodtool pciutils dwarves gcc make elfutils-libelf-devel rpm-build xz kmod \
  "xorg-x11-drv-nvidia-kmodsrc-3:$driver"
dnf download -q --destdir "$work/akmod" "akmod-nvidia-3:$driver"
akmod=$(echo "$work"/akmod/akmod-nvidia-*.rpm)
rpmkeys --checksig "$akmod" | grep -q 'signatures OK' || die "$akmod is not signed by RPM Fusion"
(cd "$work/akmod" && rpm2cpio "$akmod" | cpio -idm --quiet)
source_rpm=$(echo "$work"/akmod/usr/src/akmods/nvidia-kmod-"$driver"-*.src.rpm)
[[ -f $source_rpm ]] || die "akmod-nvidia $driver carries no module source"

# One kernel per build, as akmods does: kmodtool signs and compresses only the
# last kernel of a multi-kernel build.
for kernel in "${kernels[@]}"; do
  rpmbuild --rebuild "$source_rpm" \
    --define "_topdir $work/build" \
    --define "kernels $kernel" \
    --with kmod_nvidia_open \
    --define '_kmodtool_signmodules 1' \
    --define "_kmodtool_signmodules_privkey $key" \
    --define "_kmodtool_signmodules_pubkey $cert"
done

stage=$work/stage
mkdir -p "$stage"
for rpm in "$work"/build/RPMS/x86_64/kmod-nvidia-*.rpm; do
  (cd "$stage" && rpm2cpio "$rpm" | cpio -idm --quiet)
done
# kmodtool installs below /lib/modules, Fedora's compatibility symlink.
if [[ -d $stage/lib ]]; then
  mkdir -p "$stage/usr"
  mv "$stage/lib" "$stage/usr/lib"
fi

for kernel in "${kernels[@]}"; do
  "$here/verify-modules.sh" "$stage/usr/lib/modules/$kernel/extra/nvidia" "$cert"
done

sources=$work/SOURCES
mkdir -p "$sources" "$work/SPECS" "$out"
tar --sort=name --owner=0 --group=0 --numeric-owner -C "$stage" -cf "$sources/ryoku-nvidia-modules-$driver.tar" usr
# The driver tarball ships no license file; the open sources carry NVIDIA's
# MIT text in their headers, so the package's COPYING is taken from one.
tar -xJf "/usr/share/nvidia-kmod-$driver/nvidia-kmod-$driver-x86_64.tar.xz" -C "$work" kernel-open/nvidia/nv.c
sed -n '2,/\*\//{/\*\//d;s/^ \* \{0,1\}//;p}' "$work/kernel-open/nvidia/nv.c" >"$sources/COPYING"
grep -q '^SPDX-License-Identifier: MIT$' "$sources/COPYING" || die 'kernel-open/nvidia/nv.c no longer carries the MIT license'
cp "$cert" "$here/80-ryoku-nvidia.preset" "$sources/"
sed -e "s/^Version:.*/Version:        $driver/" \
    -e "s/^Release:.*/Release:        $revision%{?dist}/" \
    -e "s/^%global kernels .*/%global kernels         ${kernels[*]}/" \
    "$here/ryoku-nvidia.spec" > "$work/SPECS/ryoku-nvidia.spec"
rpmbuild --define "_topdir $work" --define "_srcrpmdir $out" -bs "$work/SPECS/ryoku-nvidia.spec"

python3 - "$out" "$driver" "$revision" "${kernels[@]}" <<'PY'
import hashlib, json, pathlib, sys
out, driver, revision, *kernels = sys.argv[1:]
out = pathlib.Path(out)
sources = {p.name: hashlib.sha256(p.read_bytes()).hexdigest() for p in sorted(out.glob('*.src.rpm'))}
data = dict(release=f'nvidia-{driver}-{revision}', version=driver, kernels=kernels, sources=sources)
(out / 'release.json').write_text(json.dumps(data, indent=2) + '\n')
PY
