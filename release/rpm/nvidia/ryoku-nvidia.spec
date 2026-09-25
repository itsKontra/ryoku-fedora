# prepare-srpm.sh rewrites Version, Release and kernels (newest first) per build.
%global kernels         0
%global nvidia_epoch    3
%global newest_kernel   %{lua: print((rpm.expand("%{kernels}"):match("%S+")))}
%global newest_evr      %{lua: print((rpm.expand("%{newest_kernel}"):gsub("%.x86_64$", "")))}
%global debug_package   %{nil}
# The modules arrive signed and compressed; any post-install rewrite, strip
# included, would drop the appended signature the kernel checks.
%global __os_install_post %{nil}

Name:           ryoku-nvidia
Version:        0
Release:        1%{?dist}
Summary:        Secure Boot signed NVIDIA open kernel modules for Ryoku
License:        MIT OR GPL-2.0-only
URL:            https://github.com/NVIDIA/open-gpu-kernel-modules
Source0:        ryoku-nvidia-modules-%{version}.tar
Source1:        ryoku-mok.der
Source2:        COPYING
Source3:        80-ryoku-nvidia.preset
ExclusiveArch:  x86_64
BuildRequires:  systemd-rpm-macros
Requires:       xorg-x11-drv-nvidia = %{nvidia_epoch}:%{version}
Requires:       ryoku-nvidia-kmod-%{newest_kernel} = %{version}-%{release}
Requires:       mokutil
Recommends:     xorg-x11-drv-nvidia-cuda = %{nvidia_epoch}:%{version}
# A newer kernel waits until a signed module for it is published.
Conflicts:      kernel-core > %{newest_evr}
Conflicts:      akmod-nvidia
Conflicts:      akmod-nvidia-open
Conflicts:      kmod-nvidia

%description
NVIDIA's open GPU kernel modules for Turing and newer GPUs, built from RPM
Fusion's driver source and signed with the Ryoku module key, so they load with
UEFI Secure Boot enabled once that key is enrolled. The RPM Fusion userspace is
held at the matching version, and kernel updates wait for a matching module.

%{lua:
local template = [[
%package -n ryoku-nvidia-kmod-@K@
Summary:        Signed NVIDIA open kernel modules for kernel @K@
Requires:       kernel-uname-r = @K@
Requires:       ryoku-nvidia
Provides:       nvidia-kmod = %{nvidia_epoch}:%{version}
Provides:       ryoku-nvidia-kmod = %{version}-%{release}

%description -n ryoku-nvidia-kmod-@K@
NVIDIA open kernel modules for kernel @K@, signed with the Ryoku module key.

%post -n ryoku-nvidia-kmod-@K@
/usr/sbin/depmod -a @K@ || :

%postun -n ryoku-nvidia-kmod-@K@
/usr/sbin/depmod -a @K@ || :

%files -n ryoku-nvidia-kmod-@K@
%dir /usr/lib/modules/@K@/extra
/usr/lib/modules/@K@/extra/nvidia/
]]
for kernel in rpm.expand("%{kernels}"):gmatch("%S+") do
  print(rpm.expand((template:gsub("@K@", kernel))))
end
}

%prep
cp %{SOURCE2} .

%build

%install
mkdir -p %{buildroot}
tar -C %{buildroot} -xf %{SOURCE0}
install -Dm644 %{SOURCE1} %{buildroot}%{_datadir}/ryoku/keys/ryoku-mok.der
install -Dm644 %{SOURCE3} %{buildroot}%{_presetdir}/80-ryoku-nvidia.preset

%post
systemctl --no-reload preset nvidia-fallback.service >/dev/null 2>&1 || :

%files
%license COPYING
%{_datadir}/ryoku/keys/ryoku-mok.der
%{_presetdir}/80-ryoku-nvidia.preset
