Name:           ryoku-shell
Version:        0.1.0
Release:        1%{?dist}
Summary:        Ryoku shell IPC daemon: supervises the Quickshell desktop
License:        GPL-3.0-or-later
URL:            https://ryoku.dev

BuildRequires:  golang
BuildRequires:  wayland-devel
BuildRequires:  wayland-protocols-devel
BuildRequires:  ffmpeg-free-devel
Requires:       quickshell
Requires:       hyprland
Requires:       ffmpeg-free
Requires:       wayland
Requires:       jq

%description
Ryoku shell IPC daemon (Go). Supervises the Quickshell desktop components,
drives wallpaper + palette, and serves the ryoku-shell control socket.

%build
cd %{repo_root}/ryoku/shell/ipc
CGO_ENABLED=0 go build -trimpath -mod=vendor -o %{_builddir}/ryoku-shell .
%{repo_root}/ryoku/shell/livewall/build.sh %{_builddir}/ryoku-livewall

%install
install -Dm755 %{_builddir}/ryoku-shell %{buildroot}%{_bindir}/ryoku-shell
install -Dm755 %{_builddir}/ryoku-livewall %{buildroot}%{_bindir}/ryoku-livewall
install -Dm755 %{repo_root}/ryoku/shell/scripts/ryoku-reload-cover %{buildroot}%{_bindir}/ryoku-reload-cover
install -Dm755 %{repo_root}/ryoku/shell/scripts/ryoku-depth %{buildroot}%{_bindir}/ryoku-depth
install -d %{buildroot}%{_datadir}/ryoku/reload-cover
cp -a %{repo_root}/ryoku/shell/quickshell/reload-cover/. %{buildroot}%{_datadir}/ryoku/reload-cover/

%files
%{_bindir}/ryoku-shell
%{_bindir}/ryoku-livewall
%{_bindir}/ryoku-reload-cover
%{_bindir}/ryoku-depth
%{_datadir}/ryoku/reload-cover
