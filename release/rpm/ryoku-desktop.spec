Name:           ryoku-desktop
Version:        0.1.0
Release:        1%{?dist}
Summary:        Ryoku desktop meta-package and system configuration
License:        GPL-3.0-or-later
URL:            https://ryoku.dev

Requires:       ryoku-shell = %{version}-%{release}
Requires:       ryoku-hub = %{version}-%{release}
Requires:       ryoku-rashin = %{version}-%{release}
Requires:       ryogami = %{version}-%{release}
Requires:       sddm-theme-ryoku = %{version}-%{release}
Requires:       hyprland
Requires:       quickshell
Requires:       kitty
Requires:       fish
Requires:       starship
Requires:       fastfetch
Requires:       adw-gtk3-theme
Requires:       papirus-icon-theme
Requires:       bibata-cursor-themes

%description
Ryoku desktop umbrella package for Fedora Linux. Provides core system configurations,
runtime dependencies, scripts, and default settings.

%install
install -d %{buildroot}%{_datadir}/ryoku/config
cp -a %{repo_root}/ryoku/hyprland %{buildroot}%{_datadir}/ryoku/config/hypr
cp -a %{repo_root}/ryoku/shell/quickshell %{buildroot}%{_datadir}/ryoku/config/quickshell
install -d %{buildroot}%{_bindir}
install -Dm755 %{repo_root}/ryoku/hyprland/scripts/* %{buildroot}%{_bindir}/

%files
%{_bindir}/*
%{_datadir}/ryoku/config
