Name:           ryoku-nvidia-support
Version:        0.1
Release:        1%{?dist}
Summary:        What RPM Fusion's NVIDIA driver leaves out on a Ryoku machine
License:        GPL-3.0-or-later
URL:            https://ryoku.dev
Source0:        ryoku-%{version}.tar.gz
BuildArch:      noarch
Requires:       akmod-nvidia
Requires:       libva-nvidia-driver
Requires(post): dracut
Requires(post): systemd
Requires(post): xorg-x11-drv-nvidia
Requires(postun): dracut

%description
Installed only on NVIDIA machines, by Ryoku's driver install. It pulls in
RPM Fusion's akmod-nvidia and the VA-API driver the compositor environment
names, keeps nouveau and nova_core out of the initramfs, and enables
nvidia-fallback.service, which loads nouveau from the root filesystem when the
NVIDIA module is missing. RPM Fusion's packaging owns everything else.

%prep
%setup -q -n ryoku-%{version}

%install
install -Dm644 fedora/system/hardware/nvidia/90-ryoku-nvidia.conf \
  %{buildroot}%{_prefix}/lib/dracut/dracut.conf.d/90-ryoku-nvidia.conf
install -Dm644 fedora/system/hardware/nvidia/80-ryoku-nvidia.preset \
  %{buildroot}%{_presetdir}/80-ryoku-nvidia.preset

%post
[ "$1" -eq 1 ] || exit 0
# nvidia-fallback.service arrived before this preset did, so its own package
# applied the distribution default, which leaves it disabled.
systemctl --no-reload preset nvidia-fallback.service >/dev/null 2>&1 || :
dracut --regenerate-all --force --quiet || :

%postun
[ "$1" -eq 0 ] || exit 0
dracut --regenerate-all --force --quiet || :

%files
%{_prefix}/lib/dracut/dracut.conf.d/90-ryoku-nvidia.conf
%{_presetdir}/80-ryoku-nvidia.preset
