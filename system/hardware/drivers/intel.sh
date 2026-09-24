#!/usr/bin/env bash
set -euo pipefail

source "$(dirname -- "${BASH_SOURCE[0]}")/common.sh"

if ! has_gpu_vendor 0x8086; then
  echo "intel.sh: no Intel GPU detected, nothing to do."
  exit 0
fi

install_pkgs mesa-dri-drivers mesa-libGL mesa-vulkan-drivers intel-gpu-firmware alsa-sof-firmware
# RPM Fusion's full media driver may already provide the same VA-API interface.
if ! rpm -q intel-media-driver >/dev/null 2>&1; then
  install_pkgs libva-intel-media-driver
fi
