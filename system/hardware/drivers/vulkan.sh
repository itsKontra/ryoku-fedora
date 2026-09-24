#!/usr/bin/env bash
set -euo pipefail

source "$(dirname -- "${BASH_SOURCE[0]}")/common.sh"

if ! compgen -G '/sys/class/drm/card[0-9]*' >/dev/null; then
  echo "vulkan.sh: no GPU detected, nothing to do."
  exit 0
fi

install_pkgs vulkan-loader
