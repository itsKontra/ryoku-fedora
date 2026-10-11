Name:           ryoku-shell
Version:        0.1
Release:        1%{?dist}
Summary:        ryoku-shell desktop component
License:        GPL-3.0-or-later
URL:            https://ryoku.dev
Source0:        ryoku-%{version}.tar.gz
%global debug_package %{nil}
BuildRequires:  bash
BuildRequires:  coreutils
BuildRequires:  findutils
BuildRequires:  golang >= 1.26.4
BuildRequires:  gcc
BuildRequires:  pkgconf-pkg-config
BuildRequires:  wayland-devel
BuildRequires:  wayland-protocols-devel
BuildRequires:  ffmpeg-free-devel
Requires:       quickshell
Requires:       ffmpeg-free
Requires:       jq
Requires:       uv
Requires:       python3
# Nomarchy's omarchy-* helpers: its IPC socket client, terminal pickers, Wi-Fi
# QR codes, notify-send, and the git clone behind `omarchy plugin add`.
Requires:       socat
Requires:       gum
Requires:       qrencode
Requires:       libnotify
Requires:       git-core
Requires:       librsvg2-tools
Requires:       xcursorgen
# ryoku-cursor-material-recolor renders from the Bibata sources it ships.
Requires:       ryoku-extras = %{version}-%{release}
# ryoku-host: the shell's service, package and time calls go through the seam.
Requires:       ryoku = %{version}-%{release}

%description
ryoku-shell, built from the shared Ryoku source and package payload recipe.

%prep
%setup -q -n ryoku-%{version}

%build
export RYOKU_PKGVER=%{version}
bash release/rpm/stage-package.sh %{name} "$PWD/stage" %{_libdir}

%install
cp -a stage/. %{buildroot}/
find %{buildroot} -type f -o -type l | sed 's|^%{buildroot}||' > rpm-files

%files -f rpm-files
