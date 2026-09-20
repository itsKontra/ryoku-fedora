#!/usr/bin/env bash
set -euo pipefail
[[ -f /etc/fedora-release && $EUID == 0 ]] || { echo 'requires a disposable Fedora root' >&2; exit 1; }
root=$(cd "$(dirname "$0")/../.." && pwd)
work=$(mktemp -d)
trap 'rm -rf "$work"; rm -f /etc/yum.repos.d/ryoku-signature-test.repo' EXIT
export GNUPGHOME="$work/gnupg"
mkdir -m700 "$GNUPGHOME"
gpg --batch --passphrase '' --quick-generate-key 'COPR fixture <copr@invalid.example>' rsa2048 sign 1d
gpg --batch --passphrase '' --quick-generate-key 'Metadata fixture <metadata@invalid.example>' rsa2048 sign 1d
copr=$(gpg --with-colons --list-secret-keys copr@invalid.example | awk -F: '$1=="fpr" {print $10; exit}')
metadata=$(gpg --with-colons --list-secret-keys metadata@invalid.example | awk -F: '$1=="fpr" {print $10; exit}')
cat > "$work/probe.spec" <<'SPEC'
Name: ryoku-signature-probe
Version: 0.1
Release: 1
Summary: Disposable signature and frozen download test
License: MIT
BuildArch: noarch
%description
Disposable signature and frozen download test.
%install
mkdir -p %{buildroot}/usr/share/ryoku-signature-probe
echo verified > %{buildroot}/usr/share/ryoku-signature-probe/result
%files
/usr/share/ryoku-signature-probe
SPEC
release=v0.0.0-alpha.1
out="$work/repository/releases/$release/44/x86_64"
mkdir -p "$out"
rpmbuild --define "_topdir $work/build" --define "_rpmdir $work/rpms" -bb "$work/probe.spec"
cp "$work/rpms/noarch/"*.rpm "$out/"
rpmsign --define "_gpg_name $copr" --addsign "$out/"*.rpm
gpg --armor --export "$copr" > "$work/copr.asc"
python3 - "$out" "$release" <<'PY'
import hashlib, json, pathlib, sys
out = pathlib.Path(sys.argv[1])
data = dict(release=sys.argv[2], version='0.1', channel='testing', commit='fixture')
data['rpms'] = {p.name: hashlib.sha256(p.read_bytes()).hexdigest() for p in out.glob('*.rpm')}
(out / 'release.json').write_text(json.dumps(data))
PY
RYOKU_COPR_FINGERPRINT="$copr" RYOKU_RPM_SIGNING_KEY="$metadata" \
  "$root/release/rpm/compose-repo.sh" "$out" "$work/copr.asc"
mkdir -p "$work/repository/channels"
ln -s "../releases/$release" "$work/repository/channels/testing"
cat > /etc/yum.repos.d/ryoku-signature-test.repo <<REPO
[ryoku-signature-test]
name=signature fixture
baseurl=file://$work/repository/channels/testing/44/x86_64
enabled=0
gpgcheck=1
repo_gpgcheck=1
gpgkey=file://$out/keys/copr.asc file://$out/keys/metadata.asc
REPO
dnf -y --repo=ryoku-signature-test install ryoku-signature-probe
test "$(cat /usr/share/ryoku-signature-probe/result)" = verified
dnf -y --repo=ryoku-signature-test remove ryoku-signature-probe
if python3 "$root/release/rpm/verify-rpms.py" "$out" "$work/copr.asc" "$metadata"; then
  echo 'incorrect public-key fingerprint was accepted' >&2
  exit 1
fi
printf 'damaged package' >> "$out/"*.rpm
if python3 "$root/release/rpm/verify-rpms.py" "$out" "$work/copr.asc" "$copr"; then
  echo 'modified RPM was accepted' >&2
  exit 1
fi
echo 'Publisher signatures and frozen package URLs verified'
