#!/usr/bin/env bash
# Ryoku's NVIDIA package for Fedora: what it ships, and what its scriptlets do
# on install, upgrade and removal. Pass a built RPM to check that one; with no
# argument the spec is built here with rpmbuild.
set -euo pipefail

root=${RYOKU_PATH:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}
name=ryoku-nvidia-support
spec="$root/fedora/packages/rpm/$name.spec"
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

fail() { printf 'fedora-nvidia-package: %s\n' "$*" >&2; exit 1; }

(($# <= 1)) || fail "usage: ${0##*/} [built.rpm]"

build_rpm() {
	local version=0.1 top="$work/rpmbuild" src="$work/ryoku-0.1"
	[[ -f $spec ]] || fail "missing spec: $spec"
	mkdir -p "$top/SOURCES" "$src/fedora/system/hardware"
	cp -a "$root/fedora/system/hardware/nvidia" "$src/fedora/system/hardware/"
	tar -C "$work" -czf "$top/SOURCES/ryoku-$version.tar.gz" "ryoku-$version"
	rpmbuild --quiet -bb --define "_topdir $top" "$spec" >"$work/rpmbuild.log" 2>&1 \
		|| { cat "$work/rpmbuild.log" >&2; fail "rpmbuild failed"; }
	find "$top/RPMS" -name "$name-*.rpm" -print -quit
}

rpm_file=${1:-$(build_rpm)}
[[ -f $rpm_file ]] || fail "no RPM to check: ${rpm_file:-none built}"

query() { rpm -qp --nosignature --qf "$1" "$rpm_file"; }

# Exactly the dracut drop-in and the preset: RPM Fusion owns the modprobe
# options, the kernel arguments, the initramfs NVIDIA policy and suspend.
mapfile -t files < <(rpm -qpl --nosignature "$rpm_file" | LC_ALL=C sort)
expected=(
	/usr/lib/dracut/dracut.conf.d/90-ryoku-nvidia.conf
	/usr/lib/systemd/system-preset/80-ryoku-nvidia.preset
)
[[ ${files[*]} == "${expected[*]}" ]] \
	|| fail "ships '${files[*]}', expected '${expected[*]}'"

mkdir -p "$work/payload"
rpm2archive - < "$rpm_file" | tar -xzf - -C "$work/payload"
dropin="$work/payload/usr/lib/dracut/dracut.conf.d/90-ryoku-nvidia.conf"
preset="$work/payload/usr/lib/systemd/system-preset/80-ryoku-nvidia.preset"
omitted=$(bash -c 'omit_drivers=; source "$1"; printf "%s\n" $omit_drivers' _ "$dropin" | LC_ALL=C sort | tr '\n' ' ')
[[ $omitted == "nouveau nova_core " ]] \
	|| fail "dracut drop-in omits '$omitted', expected nouveau and nova_core"
grep -vE '^[[:space:]]*(#|$)' "$preset" | grep -qxF 'enable nvidia-fallback.service' \
	|| fail "preset does not enable nvidia-fallback.service"
[[ $(grep -cvE '^[[:space:]]*(#|$)' "$preset") == 1 ]] \
	|| fail "preset carries more than the nvidia-fallback.service line"

requires=$(rpm -qp --nosignature --requires "$rpm_file" | awk '{ print $1 }')
for dep in akmod-nvidia libva-nvidia-driver; do
	grep -qxF "$dep" <<< "$requires" || fail "does not require $dep"
done

# Scriptlets run with dracut and systemctl stubbed to a call log.
mkdir -p "$work/bin"
for tool in dracut systemctl; do
	printf '#!/bin/sh\nprintf "%%s %%s\\n" %s "$*" >> "$CALLS"\n' "$tool" > "$work/bin/$tool"
	chmod 755 "$work/bin/$tool"
done
export CALLS="$work/calls"

run_scriptlet() {
	local tag=$1 arg=$2 body
	body=$(query "%{$tag}")
	[[ $body == '(none)' ]] && return 0
	PATH="$work/bin:$PATH" sh -c "$body" "$tag" "$arg"
}

transaction() {
	: > "$CALLS"
	local step
	for step in "$@"; do run_scriptlet "${step%:*}" "${step#*:}"; done
}

rebuilds() { grep -c '^dracut .*--regenerate-all' "$CALLS" || true; }

transaction PREIN:1 POSTIN:1 POSTTRANS:0
[[ $(rebuilds) == 1 ]] || fail "install rebuilt the initramfs $(rebuilds) times, expected once"
grep -q '^systemctl .*preset .*nvidia-fallback.service' "$CALLS" \
	|| fail "install did not apply the nvidia-fallback.service preset"

transaction PREIN:2 POSTIN:2 PREUN:1 POSTUN:1 POSTTRANS:0
[[ $(rebuilds) == 0 ]] || fail "upgrade rebuilt the initramfs"

transaction PREUN:0 POSTUN:0
[[ $(rebuilds) == 1 ]] || fail "removal rebuilt the initramfs $(rebuilds) times, expected once"

echo "fedora-nvidia-package: $name ships its drop-in and preset and rebuilds the initramfs on install and removal"
