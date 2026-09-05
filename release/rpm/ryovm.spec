Name:           ryovm
Version:        0.1.0
Release:        1%{?dist}
Summary:        Ryoku virtual machine manager and launcher
License:        GPL-3.0-or-later
URL:            https://ryoku.dev

Requires:       qemu-kvm
Requires:       spice-gtk
Requires:       libisoburn
Requires:       jq

%description
RyoVM manages lightweight instant VMs and GPU passthrough setups in Ryoku.

%install
install -Dm755 %{repo_root}/ryoku/apps/ryovm/bin/ryovm %{buildroot}%{_bindir}/ryovm
install -d %{buildroot}%{_datadir}/ryoku/apps/ryovm
cp -a %{repo_root}/ryoku/apps/ryovm/quickshell/. %{buildroot}%{_datadir}/ryoku/apps/ryovm/

%files
%{_bindir}/ryovm
%{_datadir}/ryoku/apps/ryovm
