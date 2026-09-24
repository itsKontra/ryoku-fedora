Name:           ryoku-desktop
Version:        0.1
Release:        1%{?dist}
Summary:        ryoku-desktop desktop component
License:        GPL-3.0-or-later
URL:            https://ryoku.dev
Source0:        ryoku-%{version}.tar.gz
%global debug_package %{nil}
BuildRequires:  bash
BuildRequires:  coreutils
BuildRequires:  findutils
BuildRequires:  golang >= 1.26.4
BuildRequires:  cmake
BuildRequires:  ninja-build
BuildRequires:  gcc-c++
BuildRequires:  qt6-qtbase-devel
BuildRequires:  qt6-qtdeclarative-devel
Requires:       ryoku = %{version}-%{release}
Requires:       ryoku-shell = %{version}-%{release}
Requires:       ryogami = %{version}-%{release}
Requires:       ryoku-hub = %{version}-%{release}
Requires:       ryoku-rashin = %{version}-%{release}
Requires:       ryoku-blobs = %{version}-%{release}
Requires:       ryoku-extras = %{version}-%{release}
Requires:       sddm-theme-ryoku = %{version}-%{release}
Requires:       ryoku-desktop-compositor
Requires:       quickshell
Requires:       sddm
Requires:       weston
Requires:       kitty
Requires:       fish
Requires:       starship
Requires:       fastfetch
Requires:       adw-gtk3-theme
Requires:       papirus-icon-theme
Requires:       gnome-keyring
Requires:       gnome-keyring-pam
Requires:       libsecret
Requires:       polkit
Requires:       xdg-desktop-portal
Requires:       xdg-desktop-portal-gtk
Requires:       qt6-qtwayland
Requires:       qt5-qtwayland
Requires:       qt6-qt5compat
Requires:       qt6-qtsvg
Requires:       qt6-qtimageformats
Requires:       qt6-qtmultimedia
Requires:       kf6-syntax-highlighting
Requires:       pipewire
Requires:       pipewire-pulseaudio
Requires:       NetworkManager
Requires:       NetworkManager-wifi
Requires:       pciutils
Requires:       usbutils
Requires:       kmod
Requires:       dracut
Requires:       grubby
Requires:       wireless-regdb
Requires:       wireplumber
Requires:       brightnessctl
Requires:       playerctl
Requires:       wl-clipboard
Requires:       cliphist
Requires:       grim
Requires:       slurp
Requires:       cava
Requires:       jq
Requires:       dnf
Requires:       rpm
Requires:       sudo
Requires:       ImageMagick
Requires:       curl
Requires:       python3
Requires:       matugen
Requires:       rsms-inter-fonts
Requires:       google-noto-sans-fonts
Requires:       google-noto-sans-cjk-fonts
Requires:       google-noto-emoji-fonts
Requires:       jetbrains-mono-fonts
Provides:       ryostore
Provides:       ryovm
Provides:       ryoku-ui = %{version}-%{release}
Obsoletes:      ryostore < 0.2
Obsoletes:      ryovm < 0.2
Obsoletes:      ryoku-ui < 0.2

Requires:       qt6ct
Requires:       zsh
Requires:       zsh-autosuggestions
Requires:       zsh-syntax-highlighting
Requires:       mpv
Requires:       mpv-mpris
Requires:       yt-dlp
Requires:       pulseaudio-utils
Requires:       pamixer
Requires:       zenity
Requires:       iw
Requires:       nftables
Requires:       bluez
Requires:       pam-u2f
Requires:       rtkit
Requires:       tesseract
Requires:       tesseract-langpack-eng
Requires:       zbar
Requires:       wf-recorder
Requires:       hyprsunset
Requires:       wtype
Requires:       libqalculate
Requires:       upower
Requires:       power-profiles-daemon
Requires:       ddcutil
Requires:       uv
Recommends:     quickemu
Recommends:     spice-gtk-tools
Recommends:     xorriso

%description
ryoku-desktop, built from the shared Ryoku source and package payload recipe.

%prep
%setup -q -n ryoku-%{version}

%build
export RYOKU_PKGVER=%{version}
bash release/rpm/stage-package.sh %{name} "$PWD/stage" %{_libdir}

%install
cp -a stage/. %{buildroot}/
find %{buildroot} -type f -o -type l | sed 's|^%{buildroot}||' > rpm-files

%post
systemctl --global enable ryoku-bootstrap.service ryoku-bluetooth-reset.service >/dev/null 2>&1 || :
systemctl daemon-reload >/dev/null 2>&1 || :
systemctl enable ryoku-wifi-regdom.service >/dev/null 2>&1 || :
/usr/bin/ryoku-bluetooth-tune >/dev/null 2>&1 || :

%files -f rpm-files
