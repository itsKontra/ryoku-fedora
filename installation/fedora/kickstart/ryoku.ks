# Fedora 44 Ryoku Desktop Anaconda Kickstart
# Single-source-of-truth installer recipe for offline media installation.

# Installer mode and graphical interface
graphical

# Keyboard, language, and timezone defaults during installation
# These defaults remain editable in Anaconda and are preserved on the target.
keyboard us
lang en_US.UTF-8
timezone UTC --utc

# Network configuration
network --bootproto=dhcp --device=link --activate

# Repository configuration: Online network sources for netinstall media
url --metalink="https://mirrors.fedoraproject.org/metalink?repo=fedora-44&arch=x86_64"
repo --name="FedoraUpdates" --metalink="https://mirrors.fedoraproject.org/metalink?repo=updates-released-f44&arch=x86_64" --cost=20
repo --name="ryoku" --baseurl="https://download.copr.fedorainfracloud.org/results/itskontra/ryoku/fedora-44-x86_64/" --cost=20 --install
repo --name="RyotunesCOPR" --baseurl="https://download.copr.fedorainfracloud.org/results/itskontra/ryotunes/fedora-44-x86_64/" --cost=20 --install
repo --name="StarshipCOPR" --baseurl="https://download.copr.fedorainfracloud.org/results/atim/starship/fedora-44-x86_64/" --cost=20 --install
repo --name="QuickshellCOPR" --baseurl="https://download.copr.fedorainfracloud.org/results/errornointernet/quickshell/fedora-44-x86_64/" --cost=20 --install
repo --name="HyprlandCOPR" --baseurl="https://download.copr.fedorainfracloud.org/results/sdegler/hyprland/fedora-44-x86_64/" --cost=20 --install
repo --name="LazygitCOPR" --baseurl="https://download.copr.fedorainfracloud.org/results/atim/lazygit/fedora-44-x86_64/" --cost=20 --install
repo --name="YaziCOPR" --baseurl="https://download.copr.fedorainfracloud.org/results/lihaohong/yazi/fedora-44-x86_64/" --cost=20 --install
repo --name="RPMFusionFree" --metalink="https://mirrors.rpmfusion.org/metalink?repo=free-fedora-44&arch=x86_64" --cost=20 --install
repo --name="RPMFusionNonfree" --metalink="https://mirrors.rpmfusion.org/metalink?repo=nonfree-fedora-44&arch=x86_64" --cost=20 --install

# Repository configuration: Local installation media with highest priority (lowest cost)
repo --name="RyokuMedia" --baseurl="file:///run/install/repo/repo" --cost=10

# Target safety: Require explicit target disk selection and user confirmation
# Unattended whole-disk wiping is strictly forbidden
clearpart --none

# Partitioning scheme (UEFI GPT)
# 1. 600 MiB FAT32 EFI System Partition
part /boot/efi --fstype="efi" --size=600
# 2. 2 GiB ext4 dedicated boot partition
part /boot --fstype="ext4" --size=2048
# 3. Remaining space as Btrfs with root, home, and snapshot subvolumes
# Optional LUKS2 encryption is prompted interactively when selected by the user
part btrfs.01 --fstype="btrfs" --size=1024 --grow
btrfs / --subvol --name=root btrfs.01
btrfs /home --subvol --name=home btrfs.01
btrfs /.snapshots --subvol --name=snapshots btrfs.01

# Bootloader configuration
bootloader --timeout=1

# Accounts and authentication
# Create your administrator account and password in Anaconda User Creation.
rootpw --lock

# System services
services --enabled="sddm,NetworkManager,firewalld,bluetooth,power-profiles-daemon,ryoku-boot-guard"

# Package payload referencing the local media repository
%packages --excludedocs
@^ryoku-desktop-environment

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
    python3 "$PROVISION_SCRIPT" /mnt/sysroot --anaconda
else
    echo "ERROR: Could not locate provision-target.py on installation media" >&2
    exit 1
fi
%end
