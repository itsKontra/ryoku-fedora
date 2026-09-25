#!/usr/bin/env bash
# Check that a directory holds NVIDIA's open modules, each signed with the
# Ryoku certificate (sign-file names the key by issuer and serial).
set -euo pipefail
dir=${1:?usage: verify-modules.sh <module directory> <certificate>}
cert=${2:?usage: verify-modules.sh <module directory> <certificate>}
die() { echo "verify-modules: $*" >&2; exit 1; }

signer=$(openssl x509 -inform DER -in "$cert" -noout -subject -nameopt multiline | sed -n 's/^ *commonName *= *//p')
serial=$(openssl x509 -inform DER -in "$cert" -noout -serial | sed 's/^serial=0*//')
count=0
for module in "$dir"/*.ko*; do
  [[ -f $module ]] || die "no modules in $dir"
  [[ $(modinfo -F signer "$module") == "$signer" ]] || die "${module##*/} is not signed by $signer"
  [[ $(modinfo -F sig_key "$module" | tr -d ':' | sed 's/^0*//') == "$serial" ]] || die "${module##*/} is signed by a different $signer key"
  count=$((count + 1))
done
(( count >= 4 )) || die "only $count modules in $dir"
# The closed build's nvidia.ko reports the NVIDIA license instead.
[[ $(modinfo -F license "$dir"/nvidia.ko*) == 'Dual MIT/GPL' ]] || die "nvidia.ko in $dir is not the open module"
echo "verify-modules: $count signed open modules in $dir"
