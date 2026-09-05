Name:           ryoku-rashin
Version:        0.1.0
Release:        1%{?dist}
Summary:        Ryoku desktop AI copilot service and tool backend
License:        GPL-3.0-or-later
URL:            https://ryoku.dev

BuildRequires:  golang

%description
Ryoku Rashin desktop AI assistant service (Go backend and skill packages).

%build
cd %{repo_root}/ryoku/rashin/backend
CGO_ENABLED=0 go build -trimpath -mod=vendor -o %{_builddir}/ryoku-rashin .

%install
install -Dm755 %{_builddir}/ryoku-rashin %{buildroot}%{_bindir}/ryoku-rashin
install -d %{buildroot}%{_datadir}/ryoku/rashin/skills
cp -a %{repo_root}/ryoku/rashin/skills/. %{buildroot}%{_datadir}/ryoku/rashin/skills/

%files
%{_bindir}/ryoku-rashin
%{_datadir}/ryoku/rashin
