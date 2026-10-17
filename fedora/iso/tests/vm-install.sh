#!/usr/bin/env bash
#
# Installs a built Ryoku Fedora ISO unattended into a QEMU VM and checks the
# result. The ISO's own kickstart is reused, made non-interactive (an
# administrator, a German keyboard to exercise the layout seed, automatic Btrfs
# partitioning, a serial console) and followed by a %post that checks what
# post/provision left behind. The install downloads every package, so it needs
# network, KVM, about 20 GB of disk and half an hour.
#
#   vm-install.sh --iso <ryoku.iso> --work <dir> [--timeout <seconds>]
#                 [--display <qemu display>]
#
# --display shows the VM (gtk, sdl); the default, none, runs it headless.
set -euo pipefail

iso='' work='' timeout=3600 display=none
while (($#)); do
  case $1 in
    --iso) iso=$2; shift 2 ;;
    --work) work=$2; shift 2 ;;
    --timeout) timeout=$2; shift 2 ;;
    --display) display=$2; shift 2 ;;
    *) echo "vm-install.sh: unknown option: $1" >&2; exit 2 ;;
  esac
done
[[ -f $iso && -n $work ]] || { echo "usage: vm-install.sh --iso <ryoku.iso> --work <dir>" >&2; exit 2; }

say() { printf ':: %s\n' "$*"; }
mkdir -p "$work"
work=$(cd "$work" && pwd)
marker=RYOKU-ISO-VM-PASS

xorriso -indev "$iso" -osirrox on -extract /ryoku.ks "$work/ryoku.ks" 2>/dev/null
sed 's/^graphical$/cmdline/' "$work/ryoku.ks" >"$work/test.ks"
cat >>"$work/test.ks" <<EOF

lang en_US.UTF-8
keyboard --vckeymap=de --xlayouts=de
timezone UTC --utc
network --bootproto=dhcp --activate
zerombr
clearpart --all --initlabel
autopart --type=btrfs
user --name=tester --groups=wheel --password=ryoku-test --plaintext
rootpw --lock
poweroff

%onerror
cat /mnt/sysroot/var/log/ryoku-iso-post.log > /dev/ttyS0 2>/dev/null
%end

%post --erroronfail
set -eu
check() { if "\$@"; then echo "ok \$*"; else echo "FAILED \$*"; exit 1; fi; }
check rpm -q ryoku ryoku-desktop ryoku-desktop-hyprland sddm-theme-ryoku
# one kernel: Anaconda installed the base from updates, so the upgrade in
# post/provision had no newer kernel to add
check test "\$(rpm -q kernel-core | wc -l)" = 1
check systemctl is-enabled --quiet sddm.service
check test "\$(systemctl get-default)" = graphical.target
check test -f /etc/yum.repos.d/ryoku.repo
check test -f /etc/snapper/configs/root
check test -f /var/lib/sddm/state.conf
check grep -q '"kbLayout": "de"' /home/tester/.config/ryoku/desktop.json
check test -d /home/tester/Pictures/Wallpapers
check test "\$(getent passwd tester | cut -d: -f7)" = /usr/bin/fish
check test "\$(stat -c %U /home/tester/.config/ryoku/desktop.json)" = tester
echo $marker > /dev/ttyS0
%end
EOF

# Anaconda draws on the last console: the window when there is one to watch,
# else the serial log.
consoles="console=tty0 console=ttyS0,115200"
[[ $display == none ]] || consoles="console=ttyS0,115200 console=tty0"

say "remastering a test ISO with the unattended kickstart"
rm -f "$work/test.iso"
mkksiso --skip-mkefiboot --ks "$work/test.ks" -r "inst.ks quiet rd.live.check" \
  -c "$consoles inst.text" \
  -R 'set default="1"' 'set default="0"' -R 'set timeout=60' 'set timeout=3' \
  "$iso" "$work/test.iso" >"$work/mkksiso.log" 2>&1

rm -f "$work/disk.qcow2" "$work/serial.log"
qemu-img create -q -f qcow2 "$work/disk.qcow2" 30G
say "installing into the VM (timeout ${timeout}s; serial log: $work/serial.log)"
set +e
timeout "$timeout" qemu-system-x86_64 \
  -enable-kvm -cpu host -smp 4 -m 4096 \
  -drive "file=$work/disk.qcow2,if=virtio" \
  -cdrom "$work/test.iso" -boot d \
  -netdev user,id=net0 -device virtio-net-pci,netdev=net0 \
  -vga virtio -display "$display" -serial "file:$work/serial.log" -no-reboot
status=$?
set -e

if grep -q "$marker" "$work/serial.log"; then
  say "the installed system passed every check"
  exit 0
fi
((status == 124)) && say "the install did not finish within ${timeout}s"
tail -n 150 "$work/serial.log" >&2
exit 1
