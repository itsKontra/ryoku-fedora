#!/usr/bin/env bash
set -euo pipefail
root=$(cd "$(dirname "$0")/../.." && pwd)
out=${RYOKU_RPM_OUT:-$root/release/rpm/out}
channel=${RYOKU_CHANNEL:-testing}
die() { echo "build-rpm-repo: $*" >&2; exit 1; }
for tool in rpmbuild createrepo_c python3 tar gzip; do command -v "$tool" >/dev/null || die "missing $tool"; done
if [[ ${RYOKU_RPM_UNSIGNED:-0} != 1 ]]; then
  : "${RYOKU_RPM_SIGNING_KEY:?set the signing key fingerprint or RYOKU_RPM_UNSIGNED=1 for local tests}"
  [[ -z $(git -C "$root" status --porcelain) ]] || die 'signed releases require a clean checkout'
fi
[[ $(git -C "$root" rev-parse --is-shallow-repository) == false ]] || die 'fetch full history for monotonic versions'
version="0.$(git -C "$root" rev-list --count HEAD)"
commit=$(git -C "$root" rev-parse HEAD)
export SOURCE_DATE_EPOCH=$(git -C "$root" show -s --format=%ct HEAD)
[[ ! -e $out ]] || die "output already exists; use a new RYOKU_RPM_OUT to retain rollback packages"
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
mkdir -p "$work/SOURCES" "$work/SPECS" "$work/tree/ryoku-$version" "$out"
# Include current edits for local builds, while excluding ignored build products.
python3 - "$root" "$work/tree/ryoku-$version" <<'PY'
import os, pathlib, shutil, subprocess, sys
root, dest = map(pathlib.Path, sys.argv[1:])
paths = subprocess.check_output(['git', '-C', str(root), 'ls-files', '-z', '--cached', '--others', '--exclude-standard']).split(b'\0')
for raw in set(paths):
    if not raw: continue
    rel = pathlib.Path(os.fsdecode(raw))
    src, dst = root / rel, dest / rel
    if not src.exists() and not src.is_symlink(): continue
    dst.parent.mkdir(parents=True, exist_ok=True)
    if src.is_symlink(): dst.symlink_to(os.readlink(src))
    elif src.is_file(): shutil.copy2(src, dst)
PY
# Resolve pinned Go dependencies into the source payload. RPM builds then run
# offline, including modules whose development checkout has no vendor tree.
while IFS= read -r -d '' gomod; do
  (cd "${gomod%/go.mod}" && GOTOOLCHAIN=local go mod vendor)
done < <(find "$work/tree/ryoku-$version/ryoku" -name vendor -prune -o -name go.mod -print0)
python3 - "$work/tree/ryoku-$version" <<'PYEXTRAS'
import importlib.machinery, pathlib, sys
root = pathlib.Path(sys.argv[1])
extra = importlib.machinery.SourceFileLoader('extra', str(root / 'ryoku/shell/scripts/ryoku-install-extra')).load_module()
cache = root / '.rpm-extras'
cache.mkdir()
for version, url, digest, relative in extra.RELEASES.values():
    (cache / digest).write_bytes(extra.download(url, digest))
PYEXTRAS
release=${RYOKU_RELEASE:-local-$version}
cat > "$work/tree/ryoku-$version/.rpm-release" <<META
RELEASE=$release
NAME=$(cat "$root/CODENAME")
CHANNEL=$channel
VERSION=$version
COMMIT=$commit
DATE=$(date -u -d "@$SOURCE_DATE_EPOCH" +%Y-%m-%dT%H:%M:%SZ)
META
tar --sort=name --mtime="@$SOURCE_DATE_EPOCH" --owner=0 --group=0 --numeric-owner -C "$work/tree" -cf - "ryoku-$version" | gzip -n > "$work/SOURCES/ryoku-$version.tar.gz"
for spec in "$root"/release/rpm/*.spec; do
  target="$work/SPECS/${spec##*/}"
  sed "s/^Version:.*/Version:        $version/" "$spec" > "$target"
  rpmbuild --define "_buildhost ryoku-builder" --define "use_source_date_epoch_as_buildtime 1" --define "clamp_mtime_to_source_date_epoch 1" --define "_topdir $work" --define "_rpmdir $out" --define "_srcrpmdir $out/SRPMS" -ba "$target"
done
mapfile -d '' rpms < <(find "$out" -name '*.rpm' ! -path '*/SRPMS/*' -print0)
((${#rpms[@]})) || die 'no RPMs built'
if [[ ${RYOKU_RPM_UNSIGNED:-0} != 1 ]]; then
  gpg --armor --export "$RYOKU_RPM_SIGNING_KEY" > "$work/key.asc"
  rpm --dbpath "$work/keydb" --import "$work/key.asc"
  for rpmfile in "${rpms[@]}"; do
    rpmsign --define "_gpg_name $RYOKU_RPM_SIGNING_KEY" --addsign "$rpmfile"
    rpmkeys --dbpath "$work/keydb" --checksig "$rpmfile" | grep -q 'signatures OK' || die "signature verification failed: $rpmfile"
  done
fi
createrepo_c --excludes 'SRPMS/*' "$out"
python3 - "$out/release.json" "$release" "$channel" "$version" "$commit" <<'PY'
import json, sys
path, release, channel, version, commit = sys.argv[1:]
with open(path, 'w') as f: json.dump(dict(release=release, channel=channel, version=version, commit=commit), f)
PY
if [[ ${RYOKU_RPM_UNSIGNED:-0} != 1 ]]; then
  gpg --batch --yes --local-user "$RYOKU_RPM_SIGNING_KEY" --armor --detach-sign "$out/repodata/repomd.xml"
  gpg --verify "$out/repodata/repomd.xml.asc" "$out/repodata/repomd.xml"
fi
printf 'Built %s (%s) at %s\n' "$version" "$commit" "$out"
