#!/usr/bin/env bash
# Enable RPM Fusion in a Fedora build container, trusting only the pinned keys.
set -euo pipefail
keys=$(cd "$(dirname "$0")/../../../installation/fedora/keys" && pwd)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
rpmkeys --import "$keys/RPM-GPG-KEY-rpmfusion-free-fedora-2020" "$keys/RPM-GPG-KEY-rpmfusion-nonfree-fedora-2020"
for repo in free nonfree; do
  curl --fail --silent --show-error --location --proto '=https' --proto-redir '=https' \
    -o "$work/rpmfusion-$repo.rpm" "https://mirrors.rpmfusion.org/$repo/fedora/rpmfusion-$repo-release-44.noarch.rpm"
  rpmkeys --checksig "$work/rpmfusion-$repo.rpm" | grep -q 'signatures OK' ||
    { echo "enable-rpmfusion: rpmfusion-$repo-release is not signed by RPM Fusion" >&2; exit 1; }
done
dnf -y install "$work"/rpmfusion-*.rpm
