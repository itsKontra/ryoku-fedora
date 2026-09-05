Name:           sddm-theme-ryoku
Version:        0.1.0
Release:        1%{?dist}
Summary:        Ryoku orbital greeter theme for SDDM
License:        GPL-3.0-or-later
URL:            https://ryoku.dev
BuildArch:      noarch

Requires:       sddm
Requires:       qt6-qtdeclarative
Requires:       qt6-qt5compat
Requires:       qt6-qtsvg
Requires:       qt6-qtmultimedia

%description
Ryoku clockwork orbital login screen theme for SDDM.

%install
install -d %{buildroot}%{_datadir}/sddm/themes/ryoku
cp -a %{repo_root}/ryoku/lockscreen/qylock/themes/clockwork/orbital/. %{buildroot}%{_datadir}/sddm/themes/ryoku/

%files
%{_datadir}/sddm/themes/ryoku
