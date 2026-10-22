#!/usr/bin/env bash
#
# Static checks for the Ryoku Fedora ISO, runnable anywhere without root or
# network: script syntax, the rendered kickstart for every Fedora release, and
# the choices, package plans and module signing steps post/provision runs on
# the installed system.
# --online also asks Fedora's repositories, through dnf5, whether every group
# and environment the kickstart names exists in each release.
#
#   check.sh [--online]
set -euo pipefail

online=0
[[ ${1:-} == --online ]] && online=1

here=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
repo=$(cd "$here/../.." && pwd)
failed=0
pass() { printf 'ok   %s\n' "$*"; }
fail() { printf 'FAIL %s\n' "$*" >&2; failed=1; }

for script in "$here"/build.sh "$here"/fetch-base.sh "$here"/post/stage "$here"/post/provision; do
  if bash -n "$script"; then pass "syntax ${script#"$repo/"}"; else fail "syntax ${script#"$repo/"}"; fi
done
if python3 -m py_compile "$here/post/check-admin" 2>/dev/null; then
  pass "syntax fedora/iso/post/check-admin"
else
  fail "syntax fedora/iso/post/check-admin"
fi
rm -rf "$here/post/__pycache__"

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
while IFS= read -r release; do
  [[ $release =~ ^[0-9]+$ ]] || continue
  volid="Ryoku-$release-x86_64"
  ((${#volid} <= 32)) || fail "volume id $volid is longer than ISO 9660 allows"
  sed -e "s/@RELEASE@/$release/g" -e "s/@VOLID@/$volid/g" "$here/kickstart/ryoku.ks" >"$work/ryoku-$release.ks"
  if grep -q '@[A-Z]*@' "$work/ryoku-$release.ks"; then
    fail "kickstart for Fedora $release keeps a placeholder"
  elif ! command -v ksvalidator >/dev/null; then
    printf 'skip kickstart validation (no ksvalidator)\n'
  elif ksvalidator -v "F$release" "$work/ryoku-$release.ks" >/dev/null 2>&1 || ksvalidator -v DEVEL "$work/ryoku-$release.ks" >/dev/null; then
    pass "kickstart for Fedora $release"
  else
    fail "kickstart for Fedora $release"
  fi
done <"$repo/fedora/releases"

if ((online)); then
  while IFS= read -r release; do
    [[ $release =~ ^[0-9]+$ ]] || continue
    while IFS= read -r group; do
      kind=group
      [[ $group == ^* ]] && kind=environment
      # dnf5 exits 0 for an unknown id too; only the printed Id proves it exists
      if dnf5 -q --releasever="$release" --repo=fedora "$kind" info "${group#^}" 2>/dev/null |
        grep -Eq "^Id +: ${group#^}$"; then
        pass "Fedora $release has $kind ${group#^}"
      else
        fail "Fedora $release has no $kind ${group#^}"
      fi
    done < <(sed -n '/^%packages/,/^%end/s/^@//p' "$here/kickstart/ryoku.ks")
  done <"$repo/fedora/releases"
fi

# The provisioner's functions, read against this checkout's tables.
# shellcheck source=/dev/null
RYOKU_ISO_PAYLOAD=$repo source "$here/post/provision"
set +e

expect() {
  local what=$1 want=$2 got=$3
  if [[ $got == "$want" ]]; then pass "$what = $want"; else fail "$what: want '$want', got '$got'"; fi
}

expect "default browser" "$(fresh_policy browser)" "$(pick browser "" "")"
expect "default login shell" "$(fresh_policy login-shell)" "$(pick login-shell "" "")"
default_compositor=$(pick compositor "" "")
if [[ -n $default_compositor ]] && available compositor "$default_compositor"; then
  pass "default compositor $default_compositor is available on Fedora"
else
  fail "no default compositor"
fi
expect "boot option beats the build default" "$default_compositor" "$(pick compositor "$default_compositor" missing-default)"
if (pick browser no-such-browser "" 2>/dev/null); then fail "an unknown choice is refused"; else pass "an unknown choice is refused"; fi
gpu_vendors="0x10de 0x8086"; expect "NVIDIA lanes" "amd intel nvidia" "$(hardware_lanes)"
gpu_vendors="0x1002"; expect "AMD lanes" "amd" "$(hardware_lanes)"
gpu_vendors="0x8086"; expect "Intel lanes" "intel" "$(hardware_lanes)"
gpu_vendors="0x1234"; expect "other lanes" "vm" "$(hardware_lanes)"

browser=$(pick browser "" "")
# shellcheck disable=SC2034 # package_plan reads it
login_shell=$(pick login-shell "" "")
while IFS= read -r compositor; do
  available compositor "$compositor" || continue
  for gpu_vendors in 0x1002 0x8086 0x10de 0x1234; do
    plan=$(package_plan)
    status=$?
    what="plan for $compositor on $gpu_vendors"
    if ((status != 0)) || [[ -z $plan ]]; then
      fail "$what does not resolve"
      continue
    fi
    missing=()
    for pkg in ryoku ryoku-desktop "ryoku-desktop-$compositor" "$browser"; do
      [[ $pkg == none ]] || grep -qx "$pkg" <<<"$plan" || missing+=("$pkg")
    done
    while IFS= read -r other; do
      [[ $other == "$compositor" ]] && continue
      grep -qx "ryoku-desktop-$other" <<<"$plan" && missing+=("no ryoku-desktop-$other")
    done < <(choice_ids compositor)
    if ((${#missing[@]})); then
      fail "$what lacks: ${missing[*]}"
    else
      pass "$what ($(wc -l <<<"$plan") packages)"
    fi
  done
done < <(choice_ids compositor)

# Module signing, against stand-ins for ryoku-host, dkms and modinfo that log
# their calls: the key comes first, DKMS builds for the kernel the machine boots
# (grubby's default, not the installer's uname -r), failures only warn, and a
# queued enrollment leaves the shell installer's notice for Anaconda.
signing=$work/signing
mkdir -p "$signing/modules/7.0.9-100.fc44.x86_64" "$signing/modules/7.0.10-100.fc44.x86_64" "$signing/modules/6.99.0-installer"
touch "$signing/modules/7.0.9-100.fc44.x86_64/vmlinuz" "$signing/modules/7.0.10-100.fc44.x86_64/vmlinuz"
boot=7.0.9-100.fc44.x86_64

# installer_notice <password>: enrollmentSummary()'s lines in the shell installer.
installer_notice() {
  sed -n '/^func enrollmentSummary/,/^}/p' "$repo/ryoku-shell-installer/fedora_modulesigning.go" |
    sed -n 's/.*i18n\.Tf\{0,1\}("\([^"]*\)".*/\1/p' | sed "s/%s/$1/"
}

# signing_run <secure boot> <failing calls>: runs sign_modules in a subshell
# and leaves its calls, output and handoff under $signing/run.
signing_run() {
  rm -rf "$signing/run"
  mkdir -p "$signing/run"
  (
    # shellcheck disable=SC2034 # provision reads them
    kernel_modules=$signing/modules handoff=$signing/run/handoff efi_dir=$signing/no-efi
    fake_secure_boot=$1 fake_failing=$2
    called() { printf '%s\n' "$*" >>"$signing/run/calls"; [[ " $fake_failing " != *" $1:$2 "* ]]; }
    uname() { printf '6.99.0-installer\n'; }
    grubby() { [[ $1 == --default-kernel ]] && printf '/boot/vmlinuz-%s\n' "$boot"; }
    mount() { called mount "$@"; }
    ryoku-host() {
      called ryoku-host "$2" || return 1
      local enrollment=not_queued
      [[ $2 == queue-enrollment && $fake_secure_boot == enforcing ]] && enrollment=queued
      printf 'action=x\nkey=present\nsecure_boot=%s\nenrollment=%s\npassword=sesame\n' "$fake_secure_boot" "$enrollment"
    }
    dkms() {
      called dkms "$@" || return 1
      [[ $1 != status ]] || printf 'hid-xpadneo/v0.10.4, %s, x86_64: installed\n' "$boot"
    }
    modinfo() { called modinfo "$@" && printf 'DKMS module signing key\n'; }
    sign_modules
  ) >"$signing/run/out" 2>&1
  printf '%s\n' "$?" >"$signing/run/status"
}

signing_run enforcing ""
expect "signing order" "ryoku-host prepare-key; dkms status -k $boot; dkms unbuild -m hid-xpadneo -v v0.10.4 -k $boot; dkms autoinstall -k $boot; modinfo -k $boot -F signer hid_xpadneo; ryoku-host queue-enrollment" \
  "$(sed -z 's/\n$//; s/\n/; /g' "$signing/run/calls" 2>/dev/null)"
expected_notice=$(installer_notice sesame)
[[ $(wc -l <<<"$expected_notice") == 2 ]] || fail "the shell installer's enrollmentSummary() has no two notice lines"
if [[ $(cat "$signing/run/handoff/enrollment-notice" 2>/dev/null) == "$expected_notice" ]]; then
  pass "the enrollment notice for Anaconda is the shell installer's"
else
  fail "the enrollment notice for Anaconda is the shell installer's"
fi
if grep -qxF "$(head -n1 <<<"$expected_notice")" "$signing/run/out"; then
  pass "the install log carries the enrollment notice"
else
  fail "the install log carries the enrollment notice"
fi
expect "ryoku-host's answer for Anaconda" "queued" "$(sed -n 's/^enrollment=//p' "$signing/run/handoff/enrollment" 2>/dev/null)"

signing_run off ""
if [[ -e $signing/run/handoff/enrollment-notice ]]; then
  fail "Secure Boot off leaves no enrollment notice"
else
  pass "Secure Boot off leaves no enrollment notice"
fi

signing_run enforcing "ryoku-host:prepare-key dkms:autoinstall"
expect "a failed key or build does not stop the install" 0 "$(cat "$signing/run/status")"
if grep -q '^ryoku-host queue-enrollment$' "$signing/run/calls"; then
  pass "the enrollment is queued after a failed build"
else
  fail "the enrollment is queued after a failed build"
fi
if grep -q 'warning:' "$signing/run/out"; then pass "a failed build warns"; else fail "a failed build warns"; fi

signing_run off "ryoku-host:queue-enrollment"
expect "a failed queue does not stop the install" 0 "$(cat "$signing/run/status")"

grubby() { return 1; }
# shellcheck disable=SC2034 # boot_kernel reads it
kernel_modules=$signing/modules
expect "without grubby, the newest installed kernel" 7.0.10-100.fc44.x86_64 "$(boot_kernel)"
unset -f grubby

if command -v shellcheck >/dev/null; then
  if shellcheck -x -S warning "$here"/build.sh "$here"/fetch-base.sh "$here"/post/stage "$here"/post/provision "$here"/tests/check.sh; then
    pass shellcheck
  else
    fail shellcheck
  fi
fi

exit "$failed"
