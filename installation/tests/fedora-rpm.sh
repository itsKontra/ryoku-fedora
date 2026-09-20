#!/usr/bin/env bash
# Run only inside a disposable Fedora container or VM.
set -euo pipefail
root=$(cd "$(dirname "$0")/../.." && pwd)
provider=${1:?compositor provider}
manager=${RYOKU_TEST_DNF:-dnf}
[[ -f /etc/fedora-release && $EUID == 0 ]] || { echo 'requires a disposable Fedora root' >&2; exit 1; }
logdir=${RYOKU_TEST_LOGS:-/tmp/fedora-evidence}
mkdir -p "$logdir"
exec > >(tee "$logdir/install.log") 2>&1
cat /etc/os-release
"$manager" --version
"$manager" -y install rpm-build rpm-sign createrepo_c python3 git gnupg2
"$root/release/rpm/enable-dependencies.sh"
if [[ ${RYOKU_TEST_PREBUILT:-0} != 1 ]]; then
  "$manager" -y install golang gcc gcc-c++ cmake ninja-build pkgconf-pkg-config \
    wayland-devel wayland-protocols-devel ffmpeg-free-devel qt6-qtbase-devel \
    qt6-qtdeclarative-devel qt6-qtmultimedia-devel qt6-qtshadertools-devel \
    qt6-qtsvg-devel qt6-qt5compat-devel qt6-qtwayland-devel
fi
export GNUPGHOME
GNUPGHOME=$(mktemp -d)
trap 'rm -rf "$GNUPGHOME"' EXIT
gpg --batch --passphrase '' --quick-generate-key 'Ryoku disposable CI <ci@invalid.example>' rsa2048 sign 1d
key=$(gpg --with-colons --list-secret-keys | awk -F: '$1=="fpr" {print $10; exit}')
out=${RYOKU_RPM_OUT:-/tmp/fedora-rpms}
if [[ ${RYOKU_TEST_SIGNED:-0} == 1 ]]; then
  [[ ${RYOKU_TEST_PREBUILT:-0} == 1 ]] || { echo 'signed verification requires prebuilt COPR output' >&2; exit 1; }
  release=$(python3 - "$out/release.json" <<'PY'
import json, re, sys
release = json.load(open(sys.argv[1]))['release']
if not re.fullmatch(r'v[0-9]+\.[0-9]+\.[0-9]+(-(alpha|beta|rc)\.[0-9]+)?', release):
    sys.exit('invalid frozen release name')
print(release)
PY
)
  fixture=$(mktemp -d)
  frozen="$fixture/releases/$release/44/x86_64"
  mkdir -p "$frozen" "$fixture/channels"
  cp -a "$out/." "$frozen/"
  ln -s "../releases/$release" "$fixture/channels/testing"
  out="$fixture/channels/testing/44/x86_64"
fi
if [[ ${RYOKU_TEST_PREBUILT:-0} != 1 ]]; then
  RYOKU_RPM_UNSIGNED=1 RYOKU_RPM_OUT="$out" "$root/release/rpm/build-rpm-repo.sh"
fi
gpg --armor --export "$key" > "$logdir/key.asc"
rpm --import "$logdir/key.asc"
if [[ ${RYOKU_TEST_SIGNED:-0} == 1 ]]; then
  : "${RYOKU_COPR_FINGERPRINT:?required for published artifact verification}"
  : "${RYOKU_RPM_SIGNING_KEY:?required for metadata verification}"
  python3 "$root/release/rpm/verify-rpms.py" "$out" "$out/keys/copr.asc" "$RYOKU_COPR_FINGERPRINT"
  python3 - "$root" "$out" "$RYOKU_RPM_SIGNING_KEY" <<'PYKEY'
import importlib.machinery, sys
verify = importlib.machinery.SourceFileLoader('verify', sys.argv[1] + '/release/rpm/verify-rpms.py').load_module()
verify.verify_key(sys.argv[2] + '/keys/metadata.asc', sys.argv[3])
PYKEY
  gpg --import "$out/keys/metadata.asc"
  gpg --verify "$out/repodata/repomd.xml.asc" "$out/repodata/repomd.xml"
  rpm --import "$out/keys/copr.asc"
  keys="file://$out/keys/copr.asc file://$out/keys/metadata.asc"
else
  while IFS= read -r -d '' package; do
    rpmsign --delsign "$package"
    rpmsign --define "_gpg_name $key" --addsign "$package"
    rpmkeys --checksig "$package"
  done < <(find "$out" -name '*.rpm' ! -path '*/SRPMS/*' -print0)
  createrepo_c --excludes 'SRPMS/*' "$out"
  gpg --batch --yes --local-user "$key" --armor --detach-sign "$out/repodata/repomd.xml"
  keys="file://$logdir/key.asc"
fi
cat > /etc/yum.repos.d/ryoku.repo <<REPO
[ryoku]
name=Ryoku test
baseurl=file://$out
enabled=1
gpgcheck=1
repo_gpgcheck=1
gpgkey=$keys
REPO
"$manager" -y install ryoku-desktop "ryoku-desktop-$provider"
testuser="ryoku-test-$provider-$$"
useradd --create-home "$testuser"
su - "$testuser" -c 'ryoku materialize'
for executable in ryoku ryoku-shell ryoku-hub ryoku-rashin ryogami ryogami-live ryostore ryovm matugen; do
  command -v "$executable"
done
"ryoku-wm-$provider" caps
for module in Ui Blobs; do test -s "/usr/lib64/qt6/qml/Ryoku/$module/qmldir"; done
test -s /usr/share/ryogami/shell.qml
test -s "/home/$testuser/.config/quickshell/hub/shell.qml"
test -s /usr/share/ryoku/i18n/en.json
rpm -qa | sort > "$logdir/packages.txt"
"$manager" repolist > "$logdir/repos.txt"
git -C "$root" rev-parse HEAD > "$logdir/commit.txt"
cp "$out/release.json" "$logdir/"
"$root/installation/tests/fedora-channels.sh" "$key"
echo 'Container build/install passed. Graphical and SELinux VM checks remain required.'
