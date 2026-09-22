#!/usr/bin/env bash
# Test harness and validation for Fedora 44 Ryoku UEFI ISO in QEMU.
# Drives automated Kickstart installation with -nic none and verifies first-boot.
set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
harness="$root/installation/tests/fedora-iso-vm.py"

ISO_PATH=""
ENCRYPTED=0
SECURE_BOOT=${RYOKU_SECURE_BOOT:-0}
STAGE_ONLY=0
DRY_RUN=0
TIMEOUT=1800
WORK_DIR=""

log() { printf '\033[1;35m::\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m:: WARNING:\033[0m %s\n' "$*" >&2; }
die() { printf 'fedora-iso-vm.sh: error: %s\n' "$*" >&2; exit 1; }

while [[ $# -gt 0 ]]; do
  case "$1" in
    --iso)
      ISO_PATH="$2"; shift 2 ;;
    --encrypted)
      ENCRYPTED=1; shift ;;
    --unencrypted)
      ENCRYPTED=0; shift ;;
    --secure-boot)
      SECURE_BOOT=1; shift ;;
    --stage-only)
      STAGE_ONLY=1; shift ;;
    --dry-run)
      DRY_RUN=1; shift ;;
    --timeout)
      TIMEOUT="$2"; shift 2 ;;
    --work-dir)
      WORK_DIR="$2"; shift 2 ;;
    -h|--help)
      cat <<'EOF'
Usage: fedora-iso-vm.sh [options]

Automated UEFI VM test harness for Fedora 44 Ryoku ISO.

Options:
  --iso <path>       Path to Fedora Ryoku ISO image
  --encrypted        Test LUKS2-encrypted installation path
  --unencrypted      Test unencrypted installation path (default)
  --secure-boot      Test with UEFI Secure Boot enabled (OVMF secboot)
  --stage-only       Validate staging, kickstart variants, and unit tests
  --dry-run          Dry-run QEMU commands without booting
  --timeout <sec>    Timeout for installer and first-boot passes (default: 1800)
  --work-dir <path>  Working directory for test artifacts
  -h, --help         Show this help message
EOF
      exit 0 ;;
    *)
      die "Unknown option: $1" ;;
  esac
done

echo "=== 1. Running Fedora VM unit tests ==="
python3 -m unittest discover -s "$root/installation/fedora/tests" -v

echo "=== 2. Validating Kickstart test variants with ksvalidator ==="
work_tmp=$(mktemp -d /tmp/vm-ks-val.XXXXXX)
trap 'rm -rf "$work_tmp"' EXIT

python3 "$harness" --dump-ks "$work_tmp/unenc.ks"
python3 "$harness" --dump-ks "$work_tmp/enc.ks" --encrypted

if command -v ksvalidator >/dev/null 2>&1; then
  ksvalidator -v F44 "$work_tmp/unenc.ks"
  echo "Unencrypted Kickstart test variant validated with ksvalidator."
  ksvalidator -v F44 "$work_tmp/enc.ks"
  echo "LUKS2-encrypted Kickstart test variant validated with ksvalidator."
else
  echo "Notice: ksvalidator not available on host; syntax validated via unit tests."
fi

echo "=== 3. Testing VM harness in stage-only mode ==="
stage_work=${WORK_DIR:-$(mktemp -d /tmp/vm-stage.XXXXXX)}
mkdir -p "$stage_work"

stage_args=(
  --stage-only
  --work-dir "$stage_work"
)
if [[ $ENCRYPTED -eq 1 ]]; then
  stage_args+=(--encrypted)
fi
if [[ $SECURE_BOOT -eq 1 ]]; then
  stage_args+=(--secure-boot)
fi

python3 "$harness" "${stage_args[@]}"

# Assert staged files
test -f "$stage_work/ks.cfg"
test -f "$stage_work/vm-provenance.json"

# Validate provenance structure
python3 -c "
import json
from pathlib import Path

prov = json.loads(Path('$stage_work/vm-provenance.json').read_text())
assert prov['harness'] == 'ryoku-fedora-iso-vm'
assert prov['status'] == 'staged'
assert '-nic none' in prov['network_policy']
if $SECURE_BOOT == 1:
    assert prov['secure_boot'] is True
print('Staged VM provenance verified successfully.')
"

if [[ $STAGE_ONLY -eq 1 ]]; then
  echo "=== Stage-only VM harness validation passed! ==="
  exit 0
fi

if [[ $DRY_RUN -eq 1 ]]; then
  echo "=== 4. Running dry-run QEMU generation ==="
  dry_run_args=(--dry-run --work-dir "$stage_work")
  if [[ $ENCRYPTED -eq 1 ]]; then
    dry_run_args+=(--encrypted)
  fi
  if [[ $SECURE_BOOT -eq 1 ]]; then
    dry_run_args+=(--secure-boot)
  fi
  python3 "$harness" "${dry_run_args[@]}"
  echo "=== Dry-run completed successfully! ==="
  exit 0
fi

echo "=== 4. Executing live UEFI VM test ==="
if [[ -z "$ISO_PATH" ]]; then
  default_iso="$root/installation/fedora/out/ryoku-fedora-44-x86_64.iso"
  if [[ -f "$default_iso" ]]; then
    ISO_PATH="$default_iso"
  else
    warn "No ISO path supplied and $default_iso not found."
    warn "To run full live VM test, build the ISO first or supply --iso <path>."
    echo "=== Stage-only checks completed. Full VM test skipped. ==="
    exit 0
  fi
fi

live_args=(
  --iso "$ISO_PATH"
  --work-dir "$stage_work"
  --timeout "$TIMEOUT"
)
if [[ $ENCRYPTED -eq 1 ]]; then
  live_args+=(--encrypted)
fi
if [[ $SECURE_BOOT -eq 1 ]]; then
  live_args+=(--secure-boot)
fi

python3 "$harness" "${live_args[@]}"

echo "=== Fedora UEFI VM test completed successfully! ==="
