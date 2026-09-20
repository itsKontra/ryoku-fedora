# Fedora 44 Ryoku Desktop Anaconda Kickstart
# Single-source-of-truth installer recipe for offline media installation.

# Installer mode and graphical interface
graphical

# Keyboard, language, and timezone defaults during installation
# (Target settings are armed and prompted on tty1 at first boot via ryoku-firstboot)
keyboard us
lang en_US.UTF-8
timezone UTC --utc

# Network configuration
network --bootproto=dhcp --device=link --activate

# Repository configuration: Local installation media with high priority (low cost)
repo --name="RyokuMedia" --baseurl="file:///run/install/repo" --cost=10

# Target safety: Require explicit target disk selection and user confirmation
# Unattended whole-disk wiping is strictly forbidden
clearpart --none

# Partitioning scheme (UEFI GPT)
# 1. 600 MiB FAT32 EFI System Partition
part /boot/efi --fstype="efi" --size=600
# 2. 2 GiB ext4 dedicated boot partition
part /boot --fstype="ext4" --size=2048
# 3. Remaining space as Btrfs with root and home subvolumes
# Optional LUKS2 encryption is prompted interactively when selected by the user
part btrfs.01 --fstype="btrfs" --size=1024 --grow
btrfs / --subvol --name=root btrfs.01
btrfs /home --subvol --name=home btrfs.01

# Bootloader configuration
bootloader --timeout=1

# Accounts and authentication
# Passwords are not embedded; accounts are locked until interactive first-boot setup
rootpw --lock
user --name=ryoku --homedir=/home/ryoku --shell=/usr/bin/fish --groups=wheel --lock

# System services
services --enabled="sddm,NetworkManager,firewalld,bluetooth"

# Package payload referencing the local media repository
%packages --excludedocs
@core
kernel
kernel-core
kernel-modules
dracut
dracut-config-generic
grub2-efi-x64
grub2-common
shim-x64
efibootmgr
btrfs-progs
dosfstools
e2fsprogs
cryptsetup
selinux-policy-targeted
setfiles
policycoreutils
policycoreutils-python-utils
shadow-utils
sudo
authselect
glibc
glibc-all-langpacks
rootfiles
util-linux
systemd
systemd-udev
systemd-resolved
systemd-networkd
kbd
kbd-misc
tzdata
dnf5
rpm
tar
gzip
iproute
iputils
chrony
mesa-dri-drivers
mesa-vulkan-drivers
vulkan-loader
xorg-x11-server-Xwayland
mesa-va-drivers
mesa-vdpau-drivers
libva-utils
linux-firmware
amd-ucode-firmware
microcode_ctl
chromium
tmux
neovim
vim-enhanced
bat
lua
python3
alacritty
fish
fzf
less
grep
ripgrep
nano
zsh
bash
firewalld
NetworkManager
NetworkManager-wifi
bluez
pipewire
wireplumber
sddm
ffmpeg
ffmpeg-libs
libavdevice
rpmfusion-free-release
rpmfusion-nonfree-release
ryoku-desktop
ryoku-desktop-niri
niri
xwayland-satellite
quickshell

# Exclude Fedora ffmpeg-free libraries so RPM Fusion full stack is guaranteed
-ffmpeg-free
-libavcodec-free
-libavdevice-free
-libavfilter-free
-libavformat-free
-libavutil-free
-libpostproc-free
-libswresample-free
-libswscale-free
%end

# Post-installation execution: Run the offline desktop provisioner
%post --nochroot --erroronfail
echo "=== Running Ryoku Desktop Offline Provisioner ==="

PROVISION_SCRIPT=""
for candidate in \
    /run/install/repo/installation/fedora/provision-target.py \
    /run/install/source/installation/fedora/provision-target.py \
    /mnt/install/source/installation/fedora/provision-target.py \
    /usr/share/ryoku/provision-target.py; do
    if [ -f "$candidate" ]; then
        PROVISION_SCRIPT="$candidate"
        break
    fi
done

if [ -z "$PROVISION_SCRIPT" ]; then
    PROVISION_SCRIPT=$(find /run/install -name "provision-target.py" 2>/dev/null | head -n 1 || true)
fi

if [ -n "$PROVISION_SCRIPT" ] && [ -f "$PROVISION_SCRIPT" ]; then
    echo "Found provisioner at: $PROVISION_SCRIPT"
    python3 "$PROVISION_SCRIPT" /mnt/sysroot
else
    echo "ERROR: Could not locate provision-target.py on installation media" >&2
    exit 1
fi
%end
