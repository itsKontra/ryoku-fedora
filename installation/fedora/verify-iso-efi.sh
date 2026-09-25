#!/usr/bin/env bash
set -euo pipefail

[[ $# == 1 && -f $1 ]] || { echo "Usage: $0 ISO" >&2; exit 1; }
for tool in xorriso mcopy; do
  command -v "$tool" >/dev/null || { echo "$tool is required" >&2; exit 1; }
done

work=$(mktemp -d /tmp/ryoku-iso-efi.XXXXXX)
trap 'rm -rf "$work"' EXIT

# USB firmware loads the FAT image, while optical boot uses the outer ISO.
xorriso -osirrox on -indev "$1" \
  -extract_boot_images "$work/boot" \
  -extract /EFI/BOOT/grub.cfg "$work/grub.cfg" \
  -extract /images/pxeboot/vmlinuz "$work/vmlinuz" \
  -extract /images/pxeboot/initrd.img "$work/initrd.img"
shopt -s nullglob
efi_images=("$work"/boot/gpt_part*_efi.img)
if [[ ${#efi_images[@]} != 1 ]]; then
  echo "Expected one GPT EFI partition in the USB image" >&2
  exit 1
fi
mcopy -i "${efi_images[0]}" ::/EFI/BOOT/grub.cfg "$work/embedded-grub.cfg"
if ! cmp -s "$work/grub.cfg" "$work/embedded-grub.cfg"; then
  echo "Embedded EFI GRUB configuration differs from the ISO; rebuild without --skip-mkefiboot" >&2
  exit 1
fi
grep -q 'inst.ks=' "$work/grub.cfg"
test -s "$work/vmlinuz"
test -s "$work/initrd.img"
echo "USB EFI configuration matches the ISO; installer kernel and initrd are present."
