#!/usr/bin/env bash
# Install built ryoku-nvidia RPMs with the RPM Fusion userspace and check that
# every module is the signed open module and that newer kernels are held back.
set -euo pipefail
rpms=$(realpath "${1:?usage: test-install.sh <directory with ryoku-nvidia RPMs>}")
here=$(cd "$(dirname "$0")" && pwd)
cert=$here/ryoku-mok.der
die() { echo "test-install: $*" >&2; exit 1; }

main=$(echo "$rpms"/ryoku-nvidia-[0-9]*.x86_64.rpm)
[[ -f $main ]] || die "no ryoku-nvidia package in $rpms"
newest=$(rpm -qp --requires "$main" | sed -n 's/^ryoku-nvidia-kmod-\(\S*\) = .*/\1/p')
[[ -n $newest ]] || die 'ryoku-nvidia does not require a module package'
rpm -qp --conflicts "$main" | grep -qx "kernel-core > ${newest%.x86_64}" ||
  die "ryoku-nvidia does not hold kernels newer than $newest"

# Scriptlets would try to build an initramfs and boot entries in the container.
dnf -y --setopt=tsflags=noscripts install "$main" "$rpms/ryoku-nvidia-kmod-$newest"-*.rpm

cmp -s "$cert" /usr/share/ryoku/keys/ryoku-mok.der || die 'the installed certificate differs from ryoku-mok.der'
"$here/verify-modules.sh" "/usr/lib/modules/$newest/extra/nvidia" "$cert"
