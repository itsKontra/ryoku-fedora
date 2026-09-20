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
"$manager" -y install rpm-build rpm-sign createrepo_c golang gcc gcc-c++ cmake ninja-build \
  pkgconf-pkg-config wayland-devel wayland-protocols-devel ffmpeg-free-devel \
  qt6-qtbase-devel qt6-qtdeclarative-devel qt6-qtmultimedia-devel \
  qt6-qtshadertools-devel qt6-qtsvg-devel qt6-qt5compat-devel qt6-qtwayland-devel \
  python3 git gnupg2 dnf5-plugins
for repo in sdegler/hyprland errornointernet/quickshell atim/starship atim/lazygit lihaohong/yazi; do
  dnf -y copr enable "$repo"
done
export GNUPGHOME
GNUPGHOME=$(mktemp -d)
trap 'rm -rf "$GNUPGHOME"' EXIT
gpg --batch --passphrase '' --quick-generate-key 'Ryoku disposable CI <ci@invalid.example>' rsa2048 sign 1d
key=$(gpg --with-colons --list-secret-keys | awk -F: '$1=="fpr" {print $10; exit}')
out=${RYOKU_RPM_OUT:-/tmp/fedora-rpms}
if [[ ${RYOKU_TEST_PREBUILT:-0} != 1 ]]; then
  RYOKU_RPM_UNSIGNED=1 RYOKU_RPM_OUT="$out" "$root/release/rpm/build-rpm-repo.sh"
fi
gpg --armor --export "$key" > "$logdir/key.asc"
rpm --import "$logdir/key.asc"
while IFS= read -r -d '' package; do
  rpmsign --delsign "$package"
  rpmsign --define "_gpg_name $key" --addsign "$package"
  rpmkeys --checksig "$package"
done < <(find "$out" -name '*.rpm' ! -path '*/SRPMS/*' -print0)
createrepo_c --excludes 'SRPMS/*' "$out"
gpg --batch --yes --local-user "$key" --armor --detach-sign "$out/repodata/repomd.xml"
cat > /etc/yum.repos.d/ryoku.repo <<REPO
[ryoku]
name=Ryoku test
baseurl=file://$out
enabled=1
gpgcheck=1
repo_gpgcheck=1
gpgkey=file://$logdir/key.asc
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
# An SRPM must unpack and rebuild with only its own source payload.
srpm=$(find "$out/SRPMS" -name 'ryoku-hub-*.src.rpm' -print -quit)
rpmbuild --define "_topdir /tmp/srpm-rebuild" --rebuild "$srpm"
"$root/installation/tests/fedora-channels.sh" "$key"
echo 'Container build/install passed. Graphical and SELinux VM checks remain required.'
