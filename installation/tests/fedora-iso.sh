#!/usr/bin/env bash
# Smoke test for Fedora 44 Anaconda Kickstart recipe and ISO compose pipeline.
set -euo pipefail

[[ ${RYOKU_TEST_DISPOSABLE:-0} == 1 && $EUID == 0 && -f /etc/fedora-release ]] || {
  echo 'requires an explicitly disposable Fedora 44 container or root' >&2
  exit 1
}

root=$(cd "$(dirname "$0")/../.." && pwd)

echo "=== 1. Running Fedora ISO unit tests ==="
python3 -m unittest discover -s "$root/installation/fedora/tests" -v

echo "=== 2. Validating Kickstart recipe with ksvalidator ==="
if ! command -v ksvalidator >/dev/null 2>&1; then
  echo "Installing pykickstart for Kickstart validation..."
  dnf -y install pykickstart >/dev/null 2>&1 || true
fi

if command -v ksvalidator >/dev/null 2>&1; then
  ksvalidator -v F44 "$root/installation/fedora/kickstart/ryoku.ks"
  echo "Kickstart syntax validated successfully with ksvalidator."
else
  echo "WARNING: ksvalidator not available; syntax checked via unit tests."
fi

echo "=== 3. Setting up mock repository for ISO compose test ==="
work=$(mktemp -d /tmp/iso-test.XXXXXX)
trap 'rm -rf "$work"' EXIT

mkdir -p "$work/repo" "$work/out" "$work/staging"

# Build a probe RPM package
cat > "$work/probe.spec" <<'SPEC'
Name:           ryoku-iso-probe
Version:        1.0
Release:        1.fc44
Summary:        Probe package for ISO compose test
License:        MIT
BuildArch:      noarch

%description
Probe package for ISO compose test.

%files
SPEC

rpmbuild --define "_topdir $work/build" --define "_rpmdir $work/repo" -bb "$work/probe.spec" >/dev/null 2>&1
mv "$work/repo/noarch/"*.rpm "$work/repo/"
rm -rf "$work/repo/noarch"

createrepo_c --checksum=sha256 "$work/repo" >/dev/null

echo "=== 4. Testing build-iso.sh in stage-only mode ==="
"$root/installation/fedora/build-iso.sh" \
  --ks "$root/installation/fedora/kickstart/ryoku.ks" \
  --packages "$root/installation/fedora/packages.list" \
  --keys-dir "$root/installation/fedora/keys" \
  --repo-dir "$work/repo" \
  --out-dir "$work/out" \
  --work-dir "$work/staging" \
  --skip-closure-verify \
  --stage-only

# Verify staged tree
test -f "$work/staging/iso_root/ryoku.ks"
test -f "$work/staging/iso_root/ks.cfg"
test -f "$work/staging/iso_root/kickstart/ryoku.ks"
test -f "$work/staging/iso_root/installation/fedora/provision-target.py"
test -f "$work/staging/iso_root/installation/fedora/prepare-firstboot.py"
test -f "$work/staging/iso_root/.ryoku-media"
test -f "$work/out/manifest.json"
test -f "$work/out/provenance.json"

echo "Staged tree and metadata verified."

echo "=== 5. Testing ISO creation with xorriso ==="
if ! command -v xorriso >/dev/null 2>&1; then
  echo "Installing xorriso..."
  dnf -y install xorriso >/dev/null 2>&1 || true
fi

if command -v xorriso >/dev/null 2>&1; then
  "$root/installation/fedora/build-iso.sh" \
    --ks "$root/installation/fedora/kickstart/ryoku.ks" \
    --packages "$root/installation/fedora/packages.list" \
    --keys-dir "$root/installation/fedora/keys" \
    --repo-dir "$work/repo" \
    --out-dir "$work/out" \
    --work-dir "$work/staging" \
    --iso-name "test-ryoku-44.iso" \
    --skip-closure-verify

  test -f "$work/out/test-ryoku-44.iso"
  test -f "$work/out/test-ryoku-44.iso.sha256"

  # Validate SHA256 checksum
  (cd "$work/out" && sha256sum -c "test-ryoku-44.iso.sha256")

  # Validate provenance records
  python3 -c "
import json
from pathlib import Path

prov = json.loads(Path('$work/out/provenance.json').read_text())
assert prov['fedora_version'] == '44'
assert prov['iso']['filename'] == 'test-ryoku-44.iso'
assert prov['iso']['size_bytes'] > 0
assert len(prov['iso']['sha256']) == 64
print('Provenance verified successfully.')
"
  echo "ISO creation and provenance verified."
else
  echo "xorriso not available; skipping full ISO packaging step."
fi

echo "=== Fedora ISO builder tests passed! ==="
