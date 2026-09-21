#!/usr/bin/env bash
# Disposable Fedora only: verify candidates and downgrades with a competing repo.
set -euo pipefail
manager=${RYOKU_TEST_DNF:-dnf}
key=${1:?signing key fingerprint}
work=$(mktemp -d)
trap 'rm -rf "$work"; rm -f /etc/yum.repos.d/ryoku-channel-test.repo' EXIT
for version in 1 2 3; do
  mkdir -p "$work/v$version"
  cat > "$work/probe.spec" <<SPEC
Name: ryoku-channel-probe
Version: $version
Release: 1
Summary: Disposable channel-selection probe
License: MIT
BuildArch: noarch
%description
Disposable test payload.
%install
mkdir -p %{buildroot}/usr/share/ryoku-channel-probe
printf '$version' > %{buildroot}/usr/share/ryoku-channel-probe/version
%files
/usr/share/ryoku-channel-probe
SPEC
  rpmbuild --define "_topdir $work/build" --define "_rpmdir $work/v$version" -bb "$work/probe.spec"
  rpmsign --define "_gpg_name $key" --addsign "$work/v$version/noarch/"*.rpm
  createrepo_c "$work/v$version"
done
# Keep the actual desktop repo intact; these isolated repo IDs have the same
# restriction semantics as the updater's --repo=RyokuCOPR transaction.
cat > /etc/yum.repos.d/ryoku-channel-test.repo <<REPO
[ryoku-test]
name=selected channel
baseurl=file://$work/v1
enabled=0
gpgcheck=1
[ryoku-competing]
name=competing channel
baseurl=file://$work/v3
enabled=1
gpgcheck=1
REPO
"$manager" -y --repo=ryoku-test install ryoku-channel-probe
sed -i "s|$work/v1|$work/v2|" /etc/yum.repos.d/ryoku-channel-test.repo
"$manager" -y --refresh --repo=ryoku-test distro-sync ryoku-channel-probe
[[ $(rpm -q --qf '%{VERSION}' ryoku-channel-probe) == 2 ]]
sed -i "s|$work/v2|$work/v1|" /etc/yum.repos.d/ryoku-channel-test.repo
"$manager" -y --refresh --repo=ryoku-test distro-sync ryoku-channel-probe
[[ $(rpm -q --qf '%{VERSION}' ryoku-channel-probe) == 1 ]]
"$manager" -y --repo=ryoku-test remove ryoku-channel-probe
