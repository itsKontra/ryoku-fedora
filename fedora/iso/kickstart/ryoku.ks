# Ryoku on Fedora @RELEASE@, installed from the Fedora netinst media.
#
# build.sh fills in @RELEASE@ and @VOLID@. Anaconda keeps every choice that is
# the user's interactive: language, keyboard, time zone, network, the target
# disk (Automatic gives the Btrfs layout in anaconda/05-ryoku.conf) and the
# administrator account. This file only fixes the package source and the
# minimal base; post/provision turns that base into Ryoku.

graphical

# The installer image carries its own disabled "updates" repo; a kickstart repo
# of that name is swallowed by it and the base installs without any updates.
url --metalink="https://mirrors.fedoraproject.org/metalink?repo=fedora-@RELEASE@&arch=x86_64"
repo --name=fedora-updates --metalink="https://mirrors.fedoraproject.org/metalink?repo=updates-released-f@RELEASE@&arch=x86_64"

%packages
@^custom-environment
@hardware-support
btrfs-progs
curl
gawk
gnupg2
policycoreutils
sudo
%end

# Find this media's ryoku-iso directory once; the later sections read the path.
%pre --erroronfail
media=
for dir in /run/install/repo /run/install/source /run/install/isodir; do
  if [ -f "$dir/ryoku-iso/ryoku-iso.conf" ]; then
    media=$dir
    break
  fi
done
if [ -z "$media" ]; then
  device=$(blkid -L '@VOLID@')
  mkdir -p /run/ryoku-media
  mount -o ro "$device" /run/ryoku-media
  media=/run/ryoku-media
fi
[ -f "$media/ryoku-iso/ryoku-iso.conf" ] || { echo "Ryoku media not found" >&2; exit 1; }
echo "$media/ryoku-iso" > /tmp/ryoku-iso-path
%end

%pre-install --erroronfail
python3 "$(cat /tmp/ryoku-iso-path)/post/check-admin"
%end

%post --nochroot --erroronfail --log=/mnt/sysroot/var/log/ryoku-iso-post.log
bash "$(cat /tmp/ryoku-iso-path)/post/stage" "${ANA_INSTALL_PATH:-/mnt/sysroot}"
%end
