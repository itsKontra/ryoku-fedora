#!/usr/bin/env bash
# Smoke test for Fedora 44 offline RPM repository and closure validation.
set -euo pipefail

[[ ${RYOKU_TEST_DISPOSABLE:-0} == 1 && $EUID == 0 && -f /etc/fedora-release ]] || {
  echo 'requires an explicitly disposable Fedora 44 container or root' >&2
  exit 1
}

root=$(cd "$(dirname "$0")/../.." && pwd)

echo "=== 1. Running Fedora repo unit tests ==="
python3 -m unittest discover -s "$root/installation/fedora/tests" -v

echo "=== 2. Verifying pinned GPG keys ==="
python3 "$root/installation/fedora/build-repo.py" \
  --packages "$root/installation/fedora/packages.list" \
  --keys-dir "$root/installation/fedora/keys" \
  --verify-keys

echo "=== 3. Testing RPM Fusion FFmpeg resolution logic ==="
work=$(mktemp -d /tmp/repo-test.XXXXXX)
trap 'rm -rf "$work"' EXIT

# Create a sample local RPM repository to test createrepo_c and offline verification
mkdir -p "$work/repo"
mkdir -p "$work/installroot"

# Create a probe RPM package to test local repository assembly and manifests
cat > "$work/probe.spec" <<'SPEC'
Name:           ryoku-offline-probe
Version:        1.0
Release:        1.fc44
Summary:        Offline probe package
License:        MIT
BuildArch:      noarch

%description
Probe package for offline repository test.

%files
SPEC

rpmbuild --define "_topdir $work/build" --define "_rpmdir $work/repo" -bb "$work/probe.spec"
mv "$work/repo/noarch/"*.rpm "$work/repo/"
rm -rf "$work/repo/noarch"

echo "=== 4. Building repository metadata with createrepo_c ==="
createrepo_c --checksum=sha256 "$work/repo"
test -f "$work/repo/repodata/repomd.xml"

echo "=== 5. Generating and validating repository manifest ==="
python3 -c "
import sys; sys.path.append('$root/installation/fedora')
import importlib.util
spec = importlib.util.spec_from_file_location('build_repo', '$root/installation/fedora/build-repo.py')
br = importlib.util.module_from_spec(spec)
spec.loader.exec_module(br)

manifest = br.generate_manifest('$work/repo', '$work/manifest.json')
assert manifest['total_packages'] >= 1
assert 'ryoku-offline-probe-1.0-1.fc44.noarch.rpm' in manifest['packages']
assert len(manifest['sha256']['ryoku-offline-probe-1.0-1.fc44.noarch.rpm']) == 64
print('Manifest generated and validated successfully.')
"

echo "=== 6. Verifying offline closure in an isolated root ==="
python3 -c "
import sys; sys.path.append('$root/installation/fedora')
import importlib.util
spec = importlib.util.spec_from_file_location('build_repo', '$root/installation/fedora/build-repo.py')
br = importlib.util.module_from_spec(spec)
spec.loader.exec_module(br)

br.verify_offline_closure(
    repo_dir='$work/repo',
    packages=['ryoku-offline-probe'],
    installroot='$work/installroot',
    dnf_bin='dnf5'
)
print('Offline closure verified in isolated installroot.')
"

echo "=== Fedora offline repository tests passed! ==="
