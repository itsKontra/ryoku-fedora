Name:           ryoku-hub
Version:        0.1.0
Release:        1%{?dist}
Summary:        Ryoku Control Hub backend and interface
License:        GPL-3.0-or-later
URL:            https://ryoku.dev

BuildRequires:  golang
Requires:       quickshell

%description
Ryoku Control Hub backend (Go) and Quickshell application interface.

%build
cd %{repo_root}/ryoku/hub/backend
CGO_ENABLED=0 go build -trimpath -mod=vendor -o %{_builddir}/ryoku-hub .

%install
install -Dm755 %{_builddir}/ryoku-hub %{buildroot}%{_bindir}/ryoku-hub
install -d %{buildroot}%{_datadir}/ryoku/hub
cp -a %{repo_root}/ryoku/hub/quickshell/. %{buildroot}%{_datadir}/ryoku/hub/

%files
%{_bindir}/ryoku-hub
%{_datadir}/ryoku/hub
