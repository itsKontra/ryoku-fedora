#!/usr/bin/env bash
# build and assemble the Fedora/RPM repository for Ryoku.
#
# Builds RPM spec files in release/rpm/ using rpmbuild, then indexes the
# produced RPM packages using createrepo_c to create an RPM repository.
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
REPO_ROOT=$(cd "$SCRIPT_DIR/../.." && pwd)
OUT_DIR=${RYOKU_RPM_OUT:-$SCRIPT_DIR/out}
ARCH=${RYOKU_RPM_ARCH:-x86_64}

say() { printf '\033[1;35m::\033[0m %s\n' "$*"; }
die() { printf 'build-rpm-repo.sh: error: %s\n' "$*" >&2; exit 1; }

command -v rpmbuild >/dev/null 2>&1 || die "rpmbuild is required (sudo dnf install rpm-build)"
command -v createrepo_c >/dev/null 2>&1 || die "createrepo_c is required (sudo dnf install createrepo_c)"

mkdir -p "$OUT_DIR/$ARCH" "$OUT_DIR/noarch" "$OUT_DIR/SRPMS" "$SCRIPT_DIR/build"

say "Building Ryoku RPM packages..."
for spec in "$SCRIPT_DIR"/*.spec; do
  [[ -f "$spec" ]] || continue
  pkg=$(basename "$spec" .spec)
  say "Building $pkg..."
  rpmbuild --define "_topdir $SCRIPT_DIR/build" \
           --define "_sourcedir $REPO_ROOT" \
           --define "_rpmdir $OUT_DIR" \
           --define "repo_root $REPO_ROOT" \
           -bb "$spec" || die "failed to build $pkg"
done

say "Generating RPM repository metadata with createrepo_c..."
createrepo_c --update "$OUT_DIR"

say "Fedora/RPM repository successfully assembled at $OUT_DIR"
