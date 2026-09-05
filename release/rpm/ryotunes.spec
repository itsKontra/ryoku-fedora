Name:           ryotunes
Version:        2.4.1
Release:        1%{?dist}
Summary:        Ryotunes: the Ryoku music app (YouTube Music client)
License:        GPL-3.0-or-later
URL:            https://github.com/neur0map/ryotunes

BuildRequires:  rust
BuildRequires:  cargo
BuildRequires:  nodejs
Requires:       webkit2gtk4.1
Requires:       gtk3
Requires:       mpv-libs
Requires:       libappindicator-gtk3

%description
Ryotunes is the Ryoku music client with native MPRIS support and wallpaper theme syncing.

%files
%{_bindir}/ryotunes
%{_datadir}/applications/ryotunes.desktop
