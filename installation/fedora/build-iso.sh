#!/usr/bin/env bash
# Compose pipeline for the Fedora 44 Ryoku Installation ISO.
#
# Orchestrates:
#   1. Verification of pinned GPG keys against fingerprints
#   2. Local repository metadata creation (createrepo_c) and SHA256 manifest generation
#   3. Offline dependency closure validation
#   4. Staging of Anaconda Kickstart, offline provisioner, and local RPM repository
#   5. mkksiso remastering of a Fedora or Lorax hybrid UEFI boot ISO
#   6. Provenance metadata and SHA256 checksum generation
#
# Usage:
#   ./build-iso.sh [options]
#
# Options:
#   --ks <path>              Path to kickstart file (default: kickstart/ryoku.ks)
#   --repo-dir <path>        Path to local RPM repository (default: /tmp/ryoku-offline-repo)
#   --packages <path>        Path to packages.list (default: packages.list)
#   --keys-dir <path>        Path to directory with pinned GPG keys (default: keys/)
#   --boot-iso <path>        Path to existing boot.iso / netinstall ISO to remaster
#   --out-dir <path>         Output directory for ISO and manifest (default: ./out or $RYOKU_ISO_OUT)
#   --work-dir <path>        Working directory for staging (default: ./work or $RYOKU_ISO_WORK)
#   --volid <string>         ISO volume label (default: Ryoku-44-x86_64)
#   --iso-name <string>      Output ISO filename (default: ryoku-fedora-44-x86_64.iso)
#   --stage-only             Prepare staging tree and provenance without building the final ISO
#   --skip-key-verify        Skip GPG key fingerprint checks
#   --verify-netinstall      Resolve the netinstall payload against Kickstart repositories
#   --skip-closure-verify    Skip isolated installroot closure resolution check
#   --cmdline <string>       Extra kernel cmdline arguments to append
#   --fast-boot              Set Grub default=0 and timeout=5 for faster/direct boot
#   --skip-mkefiboot         Skip rebuilding the EFI boot image (safe when remastering an existing ISO)
#   -h, --help               Show this help message
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
REPO_ROOT=$(cd "$SCRIPT_DIR/../.." && pwd)

# Defaults
KS_FILE="$SCRIPT_DIR/kickstart/ryoku.ks"
PACKAGES_LIST="$SCRIPT_DIR/packages.list"
KEYS_DIR="$SCRIPT_DIR/keys"
REPO_DIR="${RYOKU_LOCAL_REPO:-/tmp/ryoku-offline-repo}"
OUT_DIR="${RYOKU_ISO_OUT:-$SCRIPT_DIR/out}"
WORK_DIR="${RYOKU_ISO_WORK:-$SCRIPT_DIR/work}"
BOOT_ISO=""
VOLID="Ryoku-44-x86_64"
ISO_NAME=""
STAGE_ONLY=0
SKIP_KEY_VERIFY=0
SKIP_CLOSURE_VERIFY=0
VERIFY_NETINSTALL=0
CMDLINE=""
FAST_BOOT=0
SKIP_MKEFIBOOT=0

log() { printf '\033[1;35m::\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m:: WARNING:\033[0m %s\n' "$*" >&2; }
die() { printf 'build-iso.sh: error: %s\n' "$*" >&2; exit 1; }

# Parse arguments
while [[ $# -gt 0 ]]; do
  case "$1" in
    --ks)
      KS_FILE="$2"; shift 2 ;;
    --repo-dir)
      REPO_DIR="$2"; shift 2 ;;
    --packages)
      PACKAGES_LIST="$2"; shift 2 ;;
    --keys-dir)
      KEYS_DIR="$2"; shift 2 ;;
    --boot-iso)
      BOOT_ISO="$2"; shift 2 ;;
    --out-dir)
      OUT_DIR="$2"; shift 2 ;;
    --work-dir)
      WORK_DIR="$2"; shift 2 ;;
    --volid)
      VOLID="$2"; shift 2 ;;
    --iso-name)
      ISO_NAME="$2"; shift 2 ;;
    --stage-only)
      STAGE_ONLY=1; shift ;;
    --skip-key-verify)
      SKIP_KEY_VERIFY=1; shift ;;
    --verify-netinstall)
      VERIFY_NETINSTALL=1; shift ;;
    --skip-closure-verify)
      SKIP_CLOSURE_VERIFY=1; shift ;;
    --cmdline)
      CMDLINE="$2"; shift 2 ;;
    --fast-boot)
      FAST_BOOT=1; shift ;;
    --skip-mkefiboot)
      SKIP_MKEFIBOOT=1; shift ;;
    -h|--help)
      grep '^#' "$0" | cut -c 3- | head -n 30
      exit 0 ;;
    *)
      die "Unknown option: $1" ;;
  esac
done

# Reproducibility anchor: pin to commit timestamp if available
if _commit_epoch=$(git -C "$REPO_ROOT" log -1 --pretty=%ct 2>/dev/null) && [[ -n $_commit_epoch ]]; then
  SOURCE_DATE_EPOCH=$_commit_epoch
fi
SOURCE_DATE_EPOCH=${SOURCE_DATE_EPOCH:-$(date +%s)}
export SOURCE_DATE_EPOCH

COMMIT_SHA=$(git -C "$REPO_ROOT" rev-parse HEAD 2>/dev/null || echo "uncommitted")
COMMIT_DATE=$(git -C "$REPO_ROOT" log -1 --pretty=%cI 2>/dev/null || date -Iseconds)
BUILD_DATE=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

if [[ -z "$ISO_NAME" ]]; then
  SHORT_SHA=$(git -C "$REPO_ROOT" rev-parse --short HEAD 2>/dev/null || echo "snapshot")
  ISO_NAME="ryoku-fedora-44-${SHORT_SHA}-x86_64.iso"
fi

FINAL_ISO="$OUT_DIR/$ISO_NAME"
ISO_STAGE="$WORK_DIR/iso_root"
MANIFEST_FILE="$OUT_DIR/manifest.json"
PROVENANCE_FILE="$OUT_DIR/provenance.json"

log "Starting Fedora 44 Ryoku ISO compose pipeline"
log "  Commit:     $COMMIT_SHA ($COMMIT_DATE)"
log "  Kickstart:  $KS_FILE"
log "  Repository: $REPO_DIR"
log "  Output ISO: $FINAL_ISO"

# Step 1: Preflight checks
[[ -f "$KS_FILE" ]] || die "Kickstart file not found: $KS_FILE"
[[ -f "$PACKAGES_LIST" ]] || die "Packages list not found: $PACKAGES_LIST"
mkdir -p "$OUT_DIR" "$WORK_DIR"

command -v python3 >/dev/null 2>&1 || die "python3 is required but not installed"

if [[ $STAGE_ONLY -eq 0 ]]; then
  command -v mkksiso >/dev/null 2>&1 || die "mkksiso (lorax) is required to preserve ISO boot metadata"
  command -v xorriso >/dev/null 2>&1 || die "xorriso is required to verify ISO boot metadata"
  if [[ $SKIP_MKEFIBOOT -eq 0 ]]; then
    [[ $EUID -eq 0 ]] || die "ISO composition requires root to update the embedded EFI boot image (use --skip-mkefiboot to skip)"
  fi
  if [[ -n "$BOOT_ISO" ]]; then
    [[ -f "$BOOT_ISO" ]] || die "Boot ISO not found: $BOOT_ISO"
  else
    command -v lorax >/dev/null 2>&1 || die "Supply --boot-iso or install lorax to generate a boot ISO"
  fi
fi

# Step 2: Key verification
if [[ $SKIP_KEY_VERIFY -eq 0 && -d "$KEYS_DIR" ]]; then
  log "Verifying pinned GPG signing keys in $KEYS_DIR..."
  python3 "$SCRIPT_DIR/build-repo.py" \
    --packages "$PACKAGES_LIST" \
    --keys-dir "$KEYS_DIR" \
    --verify-keys
fi

# Step 3: Local repository metadata & manifest
if [[ -d "$REPO_DIR" ]]; then
  if command -v createrepo_c >/dev/null 2>&1; then
    log "Updating repository metadata and Ryoku Desktop environment..."
    python3 "$SCRIPT_DIR/build-repo.py" --packages "$PACKAGES_LIST" \
      --dest "$REPO_DIR" --create-repo
  elif [[ $STAGE_ONLY -eq 1 ]]; then
    warn "createrepo_c unavailable; stage-only output has no refreshed group metadata"
  else
    die "createrepo_c is required to include the Ryoku Desktop environment"
  fi

  log "Generating repository manifest..."
  python3 "$SCRIPT_DIR/build-repo.py" \
    --packages "$PACKAGES_LIST" \
    --keys-dir "$KEYS_DIR" \
    --dest "$REPO_DIR" \
    --manifest "$MANIFEST_FILE"

  if [[ $SKIP_CLOSURE_VERIFY -eq 0 && -f "$REPO_DIR/repodata/repomd.xml" ]]; then
    if command -v dnf5 >/dev/null 2>&1 || command -v dnf >/dev/null 2>&1; then
      log "Verifying offline closure in isolated installroot..."
      python3 "$SCRIPT_DIR/build-repo.py" \
        --dest "$REPO_DIR" \
        --packages "$PACKAGES_LIST" \
        --verify-closure
    fi
  fi
else
  [[ $STAGE_ONLY -eq 1 ]] || die "Repository directory does not exist: $REPO_DIR"
  warn "Repository directory $REPO_DIR does not exist yet; creating empty placeholder for staging"
  mkdir -p "$REPO_DIR"
  echo '{"total_packages": 0, "packages": {}, "sha256": {}}' > "$MANIFEST_FILE"
fi

if [[ $VERIFY_NETINSTALL -eq 1 ]]; then
  log "Resolving the complete netinstall payload against Kickstart repositories..."
  python3 "$SCRIPT_DIR/verify-netinstall.py" --ks "$KS_FILE" \
    --repo-dir "$REPO_DIR" --packages "$PACKAGES_LIST"
fi

# Step 4: Staging installer payload
log "Staging ISO payload at $ISO_STAGE..."
rm -rf "$ISO_STAGE"
mkdir -p "$ISO_STAGE"

# Inject Kickstart
mkdir -p "$ISO_STAGE/kickstart"
cp -f "$KS_FILE" "$ISO_STAGE/kickstart/ryoku.ks"
cp -f "$KS_FILE" "$ISO_STAGE/ryoku.ks"
cp -f "$KS_FILE" "$ISO_STAGE/ks.cfg"

# Inject offline provisioner and helpers
mkdir -p "$ISO_STAGE/installation/fedora"
cp -f "$SCRIPT_DIR/provision-target.py" "$ISO_STAGE/installation/fedora/"
cp -f "$SCRIPT_DIR/prepare-firstboot.py" "$ISO_STAGE/installation/fedora/"
cp -f "$SCRIPT_DIR/firstboot.py" "$ISO_STAGE/installation/fedora/"
cp -f "$SCRIPT_DIR/ryoku-firstboot.service" "$ISO_STAGE/installation/fedora/"
cp -f "$SCRIPT_DIR/firstboot-gate.conf" "$ISO_STAGE/installation/fedora/"
cp -f "$PACKAGES_LIST" "$ISO_STAGE/installation/fedora/"
if [[ -d "$KEYS_DIR" ]]; then
  cp -rf "$KEYS_DIR" "$ISO_STAGE/installation/fedora/"
fi

# Inject local RPM repository
mkdir -p "$ISO_STAGE/repo"
if [[ -d "$REPO_DIR" ]]; then
  cp -a "$REPO_DIR"/* "$ISO_STAGE/repo/" 2>/dev/null || true
fi

# Inject Ryoku payload assets and configurations for provisioner fallback
mkdir -p "$ISO_STAGE/ryoku/assets"
if [[ -d "$REPO_ROOT/ryoku/assets/wallpapers" ]]; then
  cp -a "$REPO_ROOT/ryoku/assets/wallpapers" "$ISO_STAGE/ryoku/assets/"
fi
if [[ -d "$REPO_ROOT/ryoku/assets/brand" ]]; then
  cp -a "$REPO_ROOT/ryoku/assets/brand" "$ISO_STAGE/ryoku/assets/"
fi
if [[ -d "$REPO_ROOT/ryoku/assets/ryodecors" ]]; then
  cp -a "$REPO_ROOT/ryoku/assets/ryodecors" "$ISO_STAGE/ryoku/assets/"
fi
if [[ -d "$REPO_ROOT/ryoku/apps" ]]; then
  mkdir -p "$ISO_STAGE/ryoku/apps/npm"
  if [[ -f "$REPO_ROOT/ryoku/apps/mimeapps.list" ]]; then
    cp -a "$REPO_ROOT/ryoku/apps/mimeapps.list" "$ISO_STAGE/ryoku/apps/"
  fi
  if [[ -f "$REPO_ROOT/ryoku/apps/npm/npmrc" ]]; then
    cp -a "$REPO_ROOT/ryoku/apps/npm/npmrc" "$ISO_STAGE/ryoku/apps/npm/"
  fi
fi
if [[ -d "$REPO_ROOT/ryoku/lockscreen/qylock" ]]; then
  mkdir -p "$ISO_STAGE/ryoku/lockscreen"
  cp -a "$REPO_ROOT/ryoku/lockscreen/qylock" "$ISO_STAGE/ryoku/lockscreen/"
  # Remove absolute symlinks (e.g. Qt imports pointing to /usr/lib/qt6/...)
  # that would be dangling on the ISO and crash mkksiso's CheckBigFiles.
  find "$ISO_STAGE/ryoku/lockscreen/qylock" -type l ! -exec test -e {} \; -delete 2>/dev/null || true
fi
if [[ -f "$REPO_ROOT/ryoku/shell/scripts/ryoku-install-extra" ]]; then
  mkdir -p "$ISO_STAGE/ryoku/shell/scripts"
  cp -a "$REPO_ROOT/ryoku/shell/scripts/ryoku-install-extra" "$ISO_STAGE/ryoku/shell/scripts/"
fi

# Payload stamp
cat > "$ISO_STAGE/.ryoku-media" <<EOF
NAME="Ryoku Fedora Installation Media"
VERSION="44"
ARCH="x86_64"
COMMIT="$COMMIT_SHA"
DATE="$COMMIT_DATE"
BUILD_TIME="$BUILD_DATE"
VOLUME_ID="$VOLID"
EOF

# Compute Kickstart SHA256
KS_SHA256=$(sha256sum "$KS_FILE" | awk '{print $1}')

# Step 5: Stop if stage-only
if [[ $STAGE_ONLY -eq 1 ]]; then
  log "Stage-only requested. Skipping ISO build."
  # Write provenance for the staged state
  python3 -c "
import json
from pathlib import Path

provenance = {
    'product': 'Ryoku Fedora Installation Media',
    'fedora_version': '44',
    'arch': 'x86_64',
    'stage_only': True,
    'source_commit': '$COMMIT_SHA',
    'source_date': '$COMMIT_DATE',
    'source_date_epoch': int('$SOURCE_DATE_EPOCH'),
    'build_timestamp': '$BUILD_DATE',
    'volume_id': '$VOLID',
    'kickstart': {
        'path': '$KS_FILE',
        'sha256': '$KS_SHA256',
    },
    'staging_path': '$ISO_STAGE',
    'manifest_path': '$MANIFEST_FILE',
}
Path('$PROVENANCE_FILE').write_text(json.dumps(provenance, indent=2) + '\n')
"
  log "Staged tree ready at: $ISO_STAGE"
  log "Provenance written to: $PROVENANCE_FILE"
  exit 0
fi

# Step 6: Preserve the source ISO's boot layout while adding the payload.
if [[ -z "$BOOT_ISO" ]]; then
  log "Building Anaconda installation tree using lorax..."
  LORAX_OUT="$WORK_DIR/lorax_out"
  rm -rf "$LORAX_OUT"
  lorax --product="Ryoku" --version="44" --release="1" \
    --source="file://$REPO_DIR" --volid="$VOLID" --nomacboot --noupdates "$LORAX_OUT"
  BOOT_ISO="$LORAX_OUT/images/boot.iso"
  [[ -f "$BOOT_ISO" ]] || die "Lorax did not produce images/boot.iso"
fi

verify_boot_metadata() {
  local iso=$1 report
  report=$(xorriso -indev "$iso" -report_el_torito plain -report_system_area plain 2>&1) \
    || die "Cannot inspect boot metadata: $iso"
  printf '%s\n' "$report" > "$2"
  grep -Eq 'El Torito boot img :.*UEFI' <<< "$report" \
    || die "ISO has no UEFI El Torito boot entry: $iso"
  grep -Eq 'System area summary:.*GPT' <<< "$report" \
    || die "ISO has no hybrid GPT boot layout: $iso"
}

verify_boot_metadata "$BOOT_ISO" "$OUT_DIR/source-boot-metadata.txt"
log "Using mkksiso to remaster $BOOT_ISO into $FINAL_ISO..."
mkksiso_args=(--ks "$ISO_STAGE/ryoku.ks" --volid "$VOLID")
for payload in installation ryoku repo .ryoku-media; do
  if [[ -e "$ISO_STAGE/$payload" ]]; then
    mkksiso_args+=(--add "$ISO_STAGE/$payload")
  fi
done
if [[ -n "$CMDLINE" ]]; then
  mkksiso_args+=(-c "$CMDLINE")
fi
if [[ $FAST_BOOT -eq 1 ]]; then
  mkksiso_args+=(
    -R 'set default="1"' 'set default="0"'
    -R 'set timeout=60' 'set timeout=5'
  )
fi
if [[ $SKIP_MKEFIBOOT -eq 1 ]]; then
  mkksiso_args+=(--skip-mkefiboot)
fi
[[ "$BOOT_ISO" -ef "$FINAL_ISO" ]] && die "Source and output ISO must be different files"
rm -f "$FINAL_ISO"
mkksiso "${mkksiso_args[@]}" "$BOOT_ISO" "$FINAL_ISO"
verify_boot_metadata "$FINAL_ISO" "$OUT_DIR/boot-metadata.txt"

# Step 7: Checksum and Provenance
[[ -f "$FINAL_ISO" ]] || die "Failed to produce ISO at $FINAL_ISO"

log "Computing SHA256 checksum for $FINAL_ISO..."
ISO_SHA256=$(sha256sum "$FINAL_ISO" | awk '{print $1}')
echo "$ISO_SHA256  $ISO_NAME" > "$FINAL_ISO.sha256"

ISO_SIZE=$(stat -c %s "$FINAL_ISO" 2>/dev/null || stat -f %z "$FINAL_ISO" 2>/dev/null || echo 0)

log "Recording build provenance in $PROVENANCE_FILE..."
python3 -c "
import json
import subprocess
from pathlib import Path

def get_tool_version(cmd):
    try:
        return subprocess.check_output(cmd, text=True, stderr=subprocess.DEVNULL).splitlines()[0].strip()
    except Exception:
        return 'not installed'

provenance = {
    'product': 'Ryoku Fedora Installation Media',
    'fedora_version': '44',
    'arch': 'x86_64',
    'source_commit': '$COMMIT_SHA',
    'source_date': '$COMMIT_DATE',
    'source_date_epoch': int('$SOURCE_DATE_EPOCH'),
    'build_timestamp': '$BUILD_DATE',
    'volume_id': '$VOLID',
    'iso': {
        'filename': '$ISO_NAME',
        'path': '$FINAL_ISO',
        'sha256': '$ISO_SHA256',
        'size_bytes': int('$ISO_SIZE'),
    },
    'kickstart': {
        'path': '$KS_FILE',
        'sha256': '$KS_SHA256',
    },
    'manifest_path': '$MANIFEST_FILE',
    'tools': {
        'python3': get_tool_version(['python3', '--version']),
        'createrepo_c': get_tool_version(['createrepo_c', '--version']),
        'xorriso': get_tool_version(['xorriso', '--version']),
        'mkksiso': get_tool_version(['mkksiso', '--version']),
        'lorax': get_tool_version(['lorax', '--version']),
    }
}
Path('$PROVENANCE_FILE').write_text(json.dumps(provenance, indent=2) + '\n')
"

log "Fedora 44 Ryoku ISO composed successfully!"
log "  ISO file:   $FINAL_ISO ($ISO_SIZE bytes)"
log "  SHA256:     $ISO_SHA256"
log "  Provenance: $PROVENANCE_FILE"
