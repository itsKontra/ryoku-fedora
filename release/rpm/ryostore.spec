Name:           ryostore
Version:        0.1.0
Release:        1%{?dist}
Summary:        Ryoku desktop app and add-on catalogue
License:        GPL-3.0-or-later
URL:            https://ryoku.dev

BuildRequires:  golang
Requires:       quickshell

%description
RyoStore is the Ryoku curated application, plugin, and rice browser.

%build
cd %{repo_root}/ryoku/apps/ryostore/backend
CGO_ENABLED=0 go build -trimpath -mod=vendor -o %{_builddir}/ryostore .

%install
install -Dm755 %{_builddir}/ryostore %{buildroot}%{_bindir}/ryostore
install -d %{buildroot}%{_datadir}/ryoku/apps/ryostore
cp -a %{repo_root}/ryoku/apps/ryostore/quickshell/. %{buildroot}%{_datadir}/ryoku/apps/ryostore/

%files
%{_bindir}/ryostore
%{_datadir}/ryoku/apps/ryostore
