#!/usr/bin/env bash
# Create the Machine Owner Key that signs every ryoku-nvidia kernel module.
# The certificate is committed beside this script; the private key never
# enters the repository and belongs only in the fedora-publish environment.
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
cert="$here/ryoku-mok.der"
key=${1:?usage: generate-mok-key.sh <private-key-output-path outside the repository>}
die() { echo "generate-mok-key: $*" >&2; exit 1; }

[[ ! -e $cert ]] || die "$cert already exists; rotating the key strands every enrolled machine"
[[ ! -e $key ]] || die "$key already exists"
root=$(git -C "$here" rev-parse --show-toplevel)
case $(realpath -m "$key") in "$root"/*) die 'write the private key outside the repository' ;; esac

config=$(mktemp)
trap 'rm -f "$config"' EXIT
# 1.3.6.1.4.1.2312.16.1.2 limits the key to kernel modules: shim refuses to
# boot a bootloader or kernel signed with it, so a leak cannot bypass boot.
cat >"$config" <<'CONF'
[ req ]
distinguished_name = dn
prompt = no
string_mask = utf8only
x509_extensions = ext

[ dn ]
O = Ryoku
CN = Ryoku NVIDIA module signing

[ ext ]
basicConstraints = critical,CA:FALSE
keyUsage = digitalSignature
extendedKeyUsage = codeSigning,1.3.6.1.4.1.2312.16.1.2
subjectKeyIdentifier = hash
authorityKeyIdentifier = keyid
CONF

umask 077
openssl req -x509 -new -nodes -utf8 -sha256 -days 36500 -batch \
  -newkey rsa:4096 -config "$config" -keyout "$key" -outform DER -out "$cert"
chmod 644 "$cert"
echo "certificate: $cert (commit it)"
echo "private key: $key"
echo "store it:    gh secret set RYOKU_NVIDIA_MOK_KEY --env fedora-publish < '$key'"
echo "then move the private key to offline storage and delete this copy."
