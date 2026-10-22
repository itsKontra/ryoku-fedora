# DKMS and akmods in the installer's target, before the first reboot

Research for [itsKontra/ryoku-fedora#201](https://github.com/itsKontra/ryoku-fedora/issues/201),
part of the map [#193](https://github.com/itsKontra/ryoku-fedora/issues/193).
State as of 2026-10-22. This note records facts and makes no choice.

Question: on Fedora 44 and 45, what has to happen in each install path for the
first boot to load signed `xpadneo` and `akmod-nvidia` modules, with the MOK
enrollment already queued? The paths are the Fedora ISO backend (Anaconda
installs a minimal Fedora, then `fedora/iso/post/provision` runs inside the
target chroot) and `ryoku-shell-installer` (converts a running Fedora).

Starting point, decided in #197: one module signing key per machine, DKMS's
`/var/lib/dkms/mok.key` and `mok.pub`; on NVIDIA machines akmods'
`/etc/pki/akmods/private/private_key.priv` and `certs/public_key.der` are links
to it. Enrollment is `mokutil --import` with password `enroll` plus
`mokutil --timeout -1` (#199).

Evidence: Fedora dist-git sources for `dkms`, `akmods` and `kmodtool`
(f44 and f45 branches), upstream `dkms` v3.4.4, `mokutil` 0.7.2, shim's
`MokManager.c`, Anaconda's `fsset.py`, RPM Fusion's NVIDIA specs and howtos,
and experiments in a rootless `fedora:44` podman container that builds a
target root with `dnf --installroot` and then works inside it with `chroot`,
the way `post/stage` does. The host (Fedora 44, kernel `7.2.9-200.fc44`,
Secure Boot on) was only read. Numbered references are at the end.

## Short answer

| | DKMS (`xpadneo-dkms`) | akmods (`akmod-nvidia`) |
|---|---|---|
| **When the key is made** | Lazily, by `dkms` itself, the first time it is about to sign: if `mok.key` or `mok.pub` is missing it runs `openssl req` (RSA 2048, 100 years, CN "DKMS module signing key", EKU codeSigning) [S1]. No unit, no package scriptlet. Never in a chroot unless signing is forced (below). | Only by `akmods-keygen@akmods-keygen.service`, pulled in by `akmods-keygen.target`, which `akmods.service` (boot) and `akmods@<kver>.service` (kernel install) want and order after [S2]. The akmod's own `%posttrans` build never makes a key. Verified: a booted systemd container ran keygen before `akmods.service` [E7]. |
| **Pre-existing key** | Used as is. Regenerated (both files) only if either file is missing; paths are tested with `-f`, so symlinks are followed [S1]. | Used as is. The unit is skipped when both links resolve to non-empty files (`ConditionFileNotEmpty=` follows links), and `kmodgenca -a` exits 0 with "EXISTING KEY PAIR" [S2][E5]. |
| **Links to DKMS's key** | n/a | Respected by keygen. **But the build signs as the unprivileged `akmods` user**, so a `0600 root:root` `mok.key` makes `sign-file` fail with "Permission denied", `brp-kmodsign` exits 1, and the whole kmod build fails. With `mok.key` `0640 root:akmods` it signs with DKMS's key [E5]. |
| **Dangling links** (DKMS key not made yet) | n/a | The unit's condition passes, `kmodgenca -a` makes a new akmods key and **replaces both links** with links to it [E6]. |
| **In a chroot** | Builds, but **does not sign** by default ("Running in chroot, modules won't be signed"), even when the key already exists. `try_sign_modules="true"` in `/etc/dkms/framework.conf.d/*.conf` makes it sign, and generate the key if missing [S1][E2][E3]. | No chroot check at all: signs if the key is present and readable by `akmods`, otherwise builds **unsigned** without an error [S3][E4][E5]. |
| **Which kernel, in a chroot** | `dkms build`/`install` with no `-k` means `uname -r`, the installer's (or live) kernel. The `xpadneo-dkms` `%post` uses no `-k`, so its build fails for lack of headers ("rc 21", swallowed by `|| :`). `dkms autoinstall -k <target kver>` works [E1]. | The akmod `%posttrans` runs `akmods` in the **background** (`nohup ... &`), which builds for `grubby --default-kernel` (the target's kernel) plus `uname -r` when that has headers [S3][S4][E4]. |
| **Already built unsigned** | Not repaired later: `autoinstall` skips a module already installed for that kernel, even with signing forced [S1][E3]. | Not repaired later: `akmods` skips a kmod whose package version matches the akmod; it never looks at signatures [S3]. |
| **Deferred to boot** | `dkms.service` (enabled by Fedora's preset) runs `dkms autoinstall --kernelver %v`, `Before=network-pre.target graphical.target`; not ordered before SDDM. Signs (not a chroot) and runs Fedora's `post_transaction`, `dracut --regenerate-all --force` [S1][S5]. | `akmods.service` (enabled by Fedora's preset) runs `akmods --from-init` for the running kernel, `Before=display-manager.service`, so SDDM waits for the build [S2]. |

`mokutil --import` and `mokutil --timeout -1` only read and write EFI variables
through libefivar, whose Linux backend is efivarfs at
`/sys/firmware/efi/efivars` [S6]. EFI variables
belong to the firmware, not to a root filesystem, so the calls work from any
root that has efivarfs mounted read-write. Anaconda mounts efivarfs into the
target on EFI systems [S7]. Not run here: an `--import` would have changed the
host's firmware state.

## The two install paths as they stand

### Fedora ISO backend (`fedora/iso`, branch `fedora/iso`)

- Anaconda installs the minimal base, kernel included, from `fedora` and
  `updates`. The kickstart's `%post --nochroot` runs `post/stage`, which runs
  `chroot "$root" /usr/bin/bash .../post/provision`
  (`fedora/iso/kickstart/ryoku.ks:50`, `fedora/iso/post/stage:54`).
- Anaconda has already bind-mounted `/dev` and `/run`, and mounted `devpts`,
  `sysfs`, `proc`, `selinuxfs`, `/tmp` and, on EFI, `efivarfs` into the
  target [S7]. PID 1 is the installer's systemd, so DKMS's
  `running_in_chroot()` is true (its mountinfo and root differ from PID 1's)
  [S1].
- `provision` runs `dnf upgrade -y --refresh` and then one `dnf install` of
  the package plan (`fedora/iso/post/provision:178`, `:180`). The plan carries
  `dkms`, `kernel-devel`, `kernel-headers` and `xpadneo-dkms` in the desktop
  tier (`fedora/packages/translations.tsv:34`, `:126`, `:282`). There is no
  NVIDIA driver step yet.
- `uname -r` is the installer's kernel. The target's kernel is whatever
  Anaconda installed from `updates`; `dkms` pulls in `kernel-devel-matched`
  for that kernel only [S5].

What this does today, reproduced in [E1]: `xpadneo-dkms`'s `%post` runs
`dkms add`, `build`, `install` with no `-k`. The build targets the installer's
kernel, finds no headers and fails; the `|| :` hides it. The target kernel was
installed before `dkms`, so DKMS's kernel-install hook never fired for it.
Result after reboot: no xpadneo module for the target kernel, no
`/var/lib/dkms/mok.key`, and nothing to enroll yet.

If `provision` instead ran `dkms autoinstall -k <target kver>` with default
settings, the module is built and installed **unsigned**, and no key is
generated [E1]. The first boot's `dkms.service` then sees the module as
installed and leaves it unsigned [E3], so it will not load under Secure Boot.
With `try_sign_modules="true"` set before that call, DKMS generates
`/var/lib/dkms/mok.key` (`0600 root:root`) and `mok.pub` in the target and
signs the module with it [E2].

For akmods in this path, measured with `akmod-v4l2loopback` from RPM Fusion
free as a small stand-in for `akmod-nvidia` (both are kmodtool packages and
take the same code path) [E4]:

- Installing `akmods` enables `akmods.service` in the target; the keygen
  units stay static or disabled and nothing runs them in the chroot.
- The akmod's `%posttrans` starts `akmods --from-akmod-posttrans` in the
  background. It built for the target kernel (`grubby --default-kernel` read
  the target's BLS entry), ran `dnf install` of the result, and took about
  8 s for this small module. RPM Fusion says the NVIDIA
  build can take up to 5 minutes [S8]. `provision` does not wait for it; with
  `akmod-nvidia` it would still be running when `provision` returns.
- With no key present, the kmod was built and installed **unsigned**, with
  no error. At first boot `akmods.service` would see an up-to-date kmod and
  not rebuild it [S3], while keygen would create a fresh, unused key.
- With the links to DKMS's key in place and `mok.key` `0640 root:akmods`,
  `akmods --rebuild` in the chroot built and signed with DKMS's key (same
  `sig_key` as the DKMS-signed xpadneo) [E5]. With `0600 root:root` it failed.
- `kernel-install`'s `95-akmodsposttrans.install` only does
  `systemctl restart akmods@<kver>.service --no-block`, which `systemctl`
  ignores in a chroot ("Running in chroot, ignoring command") [S2][E7].
- RPM Fusion's runtime choice between open and closed NVIDIA modules runs
  `lspci` at build time [S9]. In Anaconda's target, `/sys` is mounted, so it
  sees the real GPU (not run here with `akmod-nvidia`).

`mokutil` in this path: the target has efivarfs at
`/sys/firmware/efi/efivars` and sysfs at `/sys`, so `mokutil --import` (writes
`MokNew`, `MokAuth`) and `--timeout -1` (writes `MokTimeout`) have what they
need [S6][S7]. `mokutil` reads the password from stdin; when stdin is not a
TTY it reads plain lines, so piped input works [S6]. Its duplicate check reads
`MokListRT` from `/sys/firmware/efi/mok-variables/`, which describes the
installer's own boot through shim on the same firmware [S6]. Not executed
from an Anaconda target here.

### `ryoku-shell-installer` (running Fedora)

- Steps on Fedora, in order: `sysupgrade` (`dnf upgrade`), `tools`,
  `payload`, `backup`, `repo`, `conflicts`, `packages`, `fonts`, `drivers`,
  `session`, `configs`, `shell`, `doctor`, `verify` (`engine.go`, the `all`
  step list). `stepDrivers` on Fedora only prints "Fedora keeps NVIDIA on
  nouveau" and returns.
- This is not a chroot, so DKMS signs and generates its key whenever it
  builds. The host shows this: `/var/lib/dkms/mok.key`, `mok.pub` and the
  xpadneo build all date from 2026-10-19 12:07, and the module's signer is
  "DKMS module signing key". The key is not enrolled on the host (only
  `fedoraca` and the fork's Ryoku NVIDIA key are), so that module would be
  rejected if loaded.
- Kernel coverage depends on whether `sysupgrade` brought a new kernel:
  - Already on the newest kernel: `xpadneo-dkms`'s `%post` builds for the
    running kernel, which is the next boot's kernel. Key and signed module
    exist when the installer finishes.
  - New kernel installed by `sysupgrade`, before `dkms` existed: the `%post`
    builds only for `uname -r`, the old running kernel, and only if that
    kernel's `kernel-devel` is installable (Fedora's `updates` keeps only the
    newest build, see #194). Nothing builds for the new kernel until
    `dkms.service` on the first boot into it, so the key may not exist when
    the installer would queue enrollment. Derived from the code and [E1];
    not run on a live system.
  - Booting an older installed kernel: DKMS built nothing for it unless
    `autoinstall_all_kernels` is set [S1]; the host has `7.2.7` and `7.2.9`
    installed and xpadneo only for `7.2.9`.
- akmods on a running system behaves as in the chroot except that systemd
  is running: the akmod's `%posttrans` build happens in the background right
  after the transaction, with whatever key exists at that moment. RPM Fusion
  says the key must exist and be imported **before** the akmod is installed,
  or the driver is disabled and the screen stays blank [S8][S10]. The
  `%posttrans` path takes no shutdown inhibitor; `akmods@.service` (the kernel
  install path) does, through `systemd-inhibit --what=idle:sleep:shutdown`
  [S2][S4].

## First boot when a build is deferred

Order on the first boot after the installer reboots, from the unit files:

1. shim finds `MokNew`, starts MokManager. With `MokTimeout` set to -1 it
   shows no countdown and waits for a key press; shim deletes `MokTimeout`
   after reading it, so the setting covers one boot [S11]. The user enrolls
   with the password; the machine resets.
2. Fedora boots. `dkms.service` runs `dkms autoinstall --kernelver <running>`.
   For a module not yet built for that kernel it builds, generates the key if
   missing, signs, installs, `modprobe`s it (Fedora sets
   `modprobe_on_install="true"`) and then runs `dracut --regenerate-all
   --force` [S1][S5]. On the host, that dracut run took 54 s at install time. The unit is
   `Before=graphical.target`, but `graphical.target` itself comes
   `After=display-manager.service` and SDDM has no ordering on `dkms.service`,
   so SDDM can start while DKMS is still working (host's `systemctl cat`).
   A key generated here was not the one enrolled in step 1, unless the
   installer had already created it.
3. `akmods-keygen.target` runs keygen if the akmods key or its links are
   missing or empty [E7]; then `akmods.service` (`Before=display-manager`)
   builds and installs any missing kmod for the running kernel, shows
   "akmod: Building ..." through plymouth, and SDDM waits for it [S2][S3].
4. If the NVIDIA module is missing or rejected: `xorg-x11-drv-nvidia`'s
   `%posttrans` put `rd.driver.blacklist=nouveau,nova_core
   modprobe.blacklist=nouveau,nova_core` on every kernel's command line
   [S12], so nouveau does not load either. `nvidia-fallback.service` would
   load nouveau (`After=akmods.service`, `Before=display-manager.service`,
   only when `/sys/module/nvidia` is absent), but neither Fedora's
   `90-default.preset` nor RPM Fusion's `70-nvidia.preset` enables it; the
   host has it enabled only through the fork's `80-ryoku-nvidia.preset`
   [S12]. What the display then runs on was not tested on hardware.

## What each path needs, as facts

These follow from the findings above; they are conditions, not a design.

- The key must exist before `mokutil --import` can queue it, and it has to be
  the key that signs the modules the first boot loads.
- In a chroot, DKMS signs only with `try_sign_modules="true"`, and it must
  build for the target kernel explicitly (`-k <kver>`); the stock `%post`
  targets the installer's kernel.
- Anything built unsigned before the key existed stays unsigned: neither
  `dkms autoinstall` nor `akmods` rebuilds an installed module because it
  lacks a signature. It has to be removed or rebuilt (`dkms remove` then
  `autoinstall`, `akmods --rebuild`).
- For akmods to sign through links to DKMS's key, the `akmods` user must be
  able to read the key file: group `akmods` with read permission (`0640
  root:akmods` worked). #197's "root-only, `0600`" contradicts this.
- The links must point at a key that already exists before akmods' keygen
  runs, or keygen replaces them with a second key.
- akmods' `%posttrans` build is asynchronous. `akmods` serialises on
  `flock -w 900` over `/run/akmods/akmods.lock`, so a foreground
  `akmods --kernels <kver> --akmod nvidia` started after it waits for the
  background one [S3].
- `mokutil --import` and `--timeout -1` need efivarfs, which Anaconda's
  target has; on the running system it is already there.

## Unverified

- No Anaconda install was run; the ISO path is reproduced with
  `dnf --installroot` plus `chroot` in a container, with `/proc`, `/sys`,
  `/dev` bound and `/run` as tmpfs. Anaconda bind-mounts the installer's
  `/run` instead.
- The efivarfs mount was read from Anaconda's `fedora-45` and `main` branches;
  the repository has no `fedora-44` branch, so the F44 installer's code was
  not read directly.
- `mokutil --import` and `--timeout -1` were not executed anywhere.
- `akmod-nvidia` itself was not built; `akmod-v4l2loopback` stood in.
- SELinux was off in the container. The targeted policy has no akmods or
  dkms file contexts, and the host labels `mok.key` `var_lib_t`; whether the
  `akmods` user can read it under enforcing SELinux at boot was not tested.
- No DKMS-generated key was enrolled and loaded on hardware here; the host's
  DKMS key is not enrolled.
- Whether a `try_sign_modules=true` environment variable (instead of a
  `framework.conf.d` file) is honoured was not tested.

## Experiments

All in a rootless `--privileged` podman container from
`registry.fedoraproject.org/fedora:44`. The target root came from
`dnf --installroot=/target --releasever=44 --use-host-config` with
`kernel-core`, `kernel-modules`, `grubby`, `systemd`, `dracut` and friends,
which installed kernel `7.2.11-200.fc44` and its BLS entry. `uname -r` inside
was the host's `7.2.9-200.fc44`. Then `/proc`, `/sys`, `/dev` were
rbind-mounted, `/run` was a tmpfs, and everything below ran under
`chroot /target`. The xpadneo source was the host's
`/usr/src/hid-xpadneo-v0.10.4`. `dkms` resolved to `3.4.4-1.fc44`, `akmods`
to `0.6.2-14.fc44`, `kmodtool` to `1.2.1-1.fc44`.

- **E1** `dnf install dkms kernel-devel gcc make openssl`, then the
  `xpadneo-dkms` `%post` verbatim: `dkms build` and `install` exit 21
  ("kernel headers for kernel 7.2.9-200.fc44.x86_64 cannot be found"); status
  "added". `dkms autoinstall --kernelver 7.2.11-200.fc44.x86_64`: "Running in
  chroot, modules won't be signed", installed, `modinfo -F signer` empty, no
  `mok.key`.
- **E2** With `try_sign_modules="true"` in `framework.conf.d`, after
  `dkms remove`: "Certificate or key are missing, generating self signed
  certificate for MOK...", "Signing module ...", signer "DKMS module signing
  key"; `mok.key` `0600 root:root`, `mok.pub` `0644`, DER, EKU Code Signing,
  valid to 2126.
- **E3** `autoinstall` with `try_sign_modules="true"` while the unsigned
  module was installed: nothing rebuilt, still unsigned. Key present but
  setting back to default, after `remove`: "Running in chroot", unsigned.
- **E4** `dnf install akmods akmod-v4l2loopback` in the chroot:
  `akmods.service` enabled, `akmods-keygen.target` static; the `%posttrans`
  `akmods` ran in the background as root, `akmodsbuild` and `rpmbuild` as
  `akmods`, built for `7.2.11` and installed
  `kmod-v4l2loopback-7.2.11-200.fc44.x86_64`; `/etc/pki/akmods/{certs,private}`
  empty; module unsigned.
- **E5** Links `private_key.priv -> /var/lib/dkms/mok.key`,
  `public_key.der -> /var/lib/dkms/mok.pub`: `kmodgenca -a` printed
  "EXISTING KEY PAIR", rc 0, links unchanged; `systemd-analyze condition` with
  the keygen unit's two conditions: "Conditions failed" (the unit would be
  skipped). As `akmods`: `test -e` 0, `test -r` 1. `akmods --rebuild` with
  `mok.key 0600 root:root`: `sign-file: ... Permission denied`,
  `brp-kmodsign: modules are unsigned!`, `Bad exit status ... (%install)`,
  build failed. After `chgrp akmods` and `chmod 0640`: built and signed,
  signer "DKMS module signing key", same `sig_key` as the DKMS build.
- **E6** Links in place, DKMS key moved away: conditions succeed;
  `kmodgenca -a` made `<host>_<epoch>_<uuid>.der/.priv` and repointed both
  links at them.
- **E7** Separate container booted with systemd (`--systemd=always`,
  `akmods` installed): at boot `akmods-keygen@akmods-keygen.service` ran
  `kmodgenca -a` and created the key and links, then `akmods.service` ran;
  `systemctl list-dependencies akmods-keygen.target` shows the template
  instanced as `akmods-keygen@akmods-keygen.service`. In the chroot,
  `systemctl show` printed "Running in chroot, ignoring command".

Fedora 45 carries the same versions as Fedora 44 on 2026-10-22: `dkms`
3.4.4, `akmods` 0.6.2, `kmodtool` 1.2.1, `mokutil` 0.7.2 (dist-git `f45`);
the `akmods` f44 and f45 trees differ only in `.gitignore` and `sources`.

## References

- **S1** dkms v3.4.4, `dkms.in`: `running_in_chroot()` l.1198,
  `prepare_mok()` l.1292 (key generation l.1362), `prepare_signing()` l.1386
  ("Running in chroot" l.1400), default kernel `uname -r` l.413,
  `modprobe_on_install` l.1961, `autoinstall()` l.3007 (skip installed
  l.3059), `post_transaction` l.3715-3741.
  <https://github.com/dkms-project/dkms/blob/v3.4.4/dkms.in>. Also
  `framework.conf` comments on `try_sign_modules` and key re-creation, and
  `dkms.service` / `40-dkms.install` from the F44 package.
- **S2** akmods dist-git f44/f45 (0.6.2): `akmods-keygen@.service`,
  `akmods-keygen.target`, `akmods.service`, `akmods@.service`,
  `95-akmodsposttrans.install`, `akmods-kmodgenca` (`check_existing_key_pair`
  l.325, `sg akmods` keygen l.507, `ln -nsf` l.545), `akmods.spec` (directory
  modes `750 root:akmods`), `README.secureboot`.
  <https://src.fedoraproject.org/rpms/akmods/tree/f44>
- **S3** `akmods` script, same tree: `exec 99>/run/akmods/akmods.lock` l.268,
  `check_default_kernel` l.291, `runuser ... akmods` l.373,
  `check_kmod_up2date` l.444, `--from-init` uses `uname -r`.
- **S4** kmodtool dist-git f44 (1.2.1): `macros.kmodtool` (sign only if both
  key paths exist, defaults under `/etc/pki/akmods`), `kmodtool-brp-kmodsign`
  (exit 1 on "modules are unsigned!"), `kmodtool-kmodtool` l.180-181 (akmod
  `%posttrans`: `nohup /usr/sbin/akmods --from-akmod-posttrans --akmod ... &`).
  <https://src.fedoraproject.org/rpms/kmodtool/tree/f44>
- **S5** dkms dist-git f44 `dkms.spec`: `modprobe_on_install="true"`,
  `post_transaction="dracut --regenerate-all --force"`,
  `Requires: (kernel-devel-matched if kernel-core)`, `%systemd_post
  dkms.service`; Fedora's `90-default.preset` enables `dkms.service` and
  `akmods.service`. <https://src.fedoraproject.org/rpms/dkms/blob/f44/f/dkms.spec>
- **S6** mokutil 0.7.2: `src/mokutil.c` `update_request()` (writes
  `MokNew`/`MokAuth`), `is_duplicate()` (reads
  `/sys/firmware/efi/mok-variables/` first), `set_timeout()` (writes
  `MokTimeout`), `efi_variables_supported()` gate; `src/util.c`
  `read_hidden_line()` (stdin, echo off only on a TTY).
  <https://github.com/lcp/mokutil/tree/0.7.2>
- **S7** Anaconda `pyanaconda/modules/storage/devicetree/fsset.py`,
  `get_system_filesystems()`: `/dev` and `/run` binds, devpts, sysfs, proc,
  selinuxfs, `/tmp`, and efivarfs on EFI (`fedora-45` l.161-169, `main`
  l.162-174). <https://github.com/rhinstaller/anaconda/blob/fedora-45/pyanaconda/modules/storage/devicetree/fsset.py>
- **S8** RPM Fusion, Howto/NVIDIA: Secure Boot key "before installing the
  kmod", "wait until the kmod has been built ... up to 5 minutes".
  <https://rpmfusion.org/Howto/NVIDIA>
- **S9** RPM Fusion `nvidia-kmod` f44: `nvidia-kmod.spec` (runtime open/closed
  detection), `nvidia-kmod-noopen-checks` (`lspci -nn`).
  <https://github.com/rpmfusion/nvidia-kmod/tree/f44>
- **S10** RPM Fusion, Howto/Secure Boot: `kmodgenca -a`, `mokutil --import
  /etc/pki/akmods/certs/public_key.der`, MokManager flow.
  <https://rpmfusion.org/Howto/Secure%20Boot>
- **S11** shim `MokManager.c`: reads `MokTimeout`, deletes it, negative
  timeout skips the countdown. <https://github.com/rhboot/shim/blob/main/MokManager.c>
- **S12** RPM Fusion `xorg-x11-drv-nvidia` f44 (615.78.08): `%posttrans`
  grubby args, `nvidia-fallback.service`, `70-nvidia.preset` (no
  `nvidia-fallback`). <https://github.com/rpmfusion/xorg-x11-drv-nvidia/tree/f44>
- Repo: `fedora/iso/kickstart/ryoku.ks`, `fedora/iso/post/stage`,
  `fedora/iso/post/provision`, `fedora/packages/translations.tsv`,
  `fedora/packages/rpm/xpadneo-dkms.spec` (branch `fedora/iso`);
  `ryoku-shell-installer/engine.go` (`stepSysupgrade`, `stepDrivers`, step
  list).
