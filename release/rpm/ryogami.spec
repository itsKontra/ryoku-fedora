Name:           ryogami
Version:        0.1.0
Release:        1%{?dist}
Summary:        Ryoku wallpaper manager and canvas engine
License:        GPL-3.0-or-later
URL:            https://ryoku.dev

BuildRequires:  golang

%description
Ryogami wallpaper manager daemon and canvas renderer for the Ryoku shell.

%build
cd %{repo_root}/ryoku/shell/ryogami
CGO_ENABLED=0 go build -trimpath -mod=vendor -o %{_builddir}/ryogami .

%install
install -Dm755 %{_builddir}/ryogami %{buildroot}%{_bindir}/ryogami

%files
%{_bindir}/ryogami
