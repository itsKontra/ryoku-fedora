#!/usr/bin/env bash
set -euo pipefail

source "$(dirname -- "${BASH_SOURCE[0]}")/common.sh"

if ! has_gpu_vendor 0x1002; then
  echo "amd.sh: no AMD GPU detected, nothing to do."
  exit 0
fi

install_pkgs mesa-dri-drivers mesa-libGL mesa-vulkan-drivers amd-gpu-firmware
