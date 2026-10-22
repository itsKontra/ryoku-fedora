# akmods and DKMS signing inside the installer's target

Research for [itsKontra/ryoku-fedora#201](https://github.com/itsKontra/ryoku-fedora/issues/201),
part of the map [#193](https://github.com/itsKontra/ryoku-fedora/issues/193).
State as of 2026-10-22.

Question: the machine's one module signing key is DKMS's `/var/lib/dkms/mok.key`,
and on NVIDIA machines akmods' key files link to it (decision in #197). On
Fedora 44 and 45, what has to happen in each install path for the first boot to
load signed `xpadneo` and `akmod-nvidia` modules, with the enrollment already
queued? The paths are the Fedora ISO, whose kickstart runs `post/provision` in a
`chroot` of Anaconda's target root, and `ryoku-shell-installer`, which converts
a running system.

Evidence comes from the shipped packages (pulled with `dnf download` for F44
and with `--releasever=45` for F45 and unpacked), their upstream sources, and
this repository. Numbered references are at the end. Line numbers refer to the
files as shipped.

## Short answer

Nothing needed is impossible in the target root, but none of it happens by
itself. Each step needs an explicit command in the installer:

- **Keys.** DKMS creates `mok.key` / `mok.pub` the first time it signs a module,
  or when `dkms generate_mok` runs. akmods' own script never creates a key. Only
  `akmods-keygen@.service`, pulled in by `akmods.service` at boot, runs
  `kmodgenca -a`, and it skips when both key paths resolve to non-empty files.
  Symlinks to the DKMS key count as present.
- **DKMS in a chroot** builds but does not sign, and does not create a key,
  unless `try_sign_modules=true`. A module installed unsigned stays unsigned:
  `dkms.service` at boot skips modules that are already installed.
- **akmods in a chroot** signs if the key files exist and the unprivileged
  `akmods` user can read them. It has no chroot check. With no key it builds
  unsigned without saying so. With a key it cannot read, the build fails and
  leaves a marker that `akmods.service` will not retry without `--force`.
- **akmods builds as the `akmods` user.** So on NVIDIA machines
  `/var/lib/dkms/mok.key` has to be `root:akmods 0640`, which is akmods' own
  convention, not the root-only `0600` that DKMS writes. This changes item 6 of
  #197.
- **`mokutil --import` and `--timeout -1` work from the target root** whenever
  efivarfs is mounted there. Anaconda 44 and 45 mount it into the sysroot on
  UEFI machines. A hand-made chroot without that mount fails with "EFI
  variables are not supported on this system".
- **If a build is deferred to first boot**, `dkms.service` and `akmods.service`
  build before the login screen and sign with the key that is already queued.
  NVIDIA adds minutes to that boot, and a missing key at that point produces a
  new key that is not enrolled. Building in the target, then checking the
  signer, is the reliable path.

## Versions checked

| Package | Fedora 44 | Fedora 45 | Differences relevant here |
|---|---|---|---|
| dkms | 3.4.4-1.fc44 (3.4.3-2 installed on the test box) | 3.4.3-2.fc45 | None: `running_in_chroot`, `prepare_mok`, `prepare_signing` and `framework.conf` are identical |
| akmods | 0.6.2-14.fc44 | 0.6.2-14.fc45 | Identical files |
| kmodtool | 1.2.1-1.fc44 | 1.2.1-2.fc45 | Mass rebuild only; `kmodtool` and `macros.kmodtool` identical |
| mokutil | 0.7.2-3.fc44 | 0.7.2-7.fc45 | OpenSSL 4.0 patch and rebuilds only |
| anaconda-core | 44.30-2.fc44 | 45.30-1.fc45 | Same system mounts (`fsset.py`) |
| akmod-nvidia (RPM Fusion) | 615.78.08-1.fc44 | (same branch, see #194) | n/a |

So everything below holds for both releases [S1][S2][S3][S4][S5].

## 1. When each tool generates a key

**DKMS.** `prepare_mok()` (dkms 1292-1384) defaults to
`mok_signing_key=/var/lib/dkms/mok.key` and `mok_certificate=/var/lib/dkms/mok.pub`
(1350-1358). If either file is missing, it runs
`openssl req -new -x509 -nodes -days 36500 -subj "/CN=DKMS module signing key" -newkey rsa:2048 ... -outform DER`
(1361-1371). That gives a PEM private key and a DER certificate, the same
formats akmods expects. `prepare_mok` is reached in only two ways:

- from `prepare_signing()` (1386-1445), which runs on each build *after* the
  policy and chroot checks, so a chroot build never creates a key.
- from the `dkms generate_mok` action (3763-3765), which has no chroot check.

On this box the key landed as `root:root 0600`, labelled `var_lib_t`, on the
day `xpadneo-dkms` was installed on the running system [S1][S6].

**akmods.** The `akmods` script contains no key generation at all. The key comes
from `kmodgenca -a`, run by `akmods-keygen@.service`
(`ExecStart=/usr/sbin/kmodgenca -a`). That unit is wanted by
`akmods-keygen.target`, which `akmods.service` and `akmods@.service` both
`Want` and order `After` [S2]. In practice the first akmods key appears at the
first boot that runs `akmods.service`, as `README.secureboot` says ("At the
first run of the akmods.service, certificate and keypair will be created") [S7].
`akmods.service` is enabled by Fedora's `90-default.preset` (`enable
akmods.service`), and so is `dkms.service` [S8].

## 2. Building and signing in a chroot, or deferring

### DKMS

`try_sign_modules` defaults to `not_in_chroot` (`/etc/dkms/framework.conf`),
and any value other than `true` acts as that default (dkms 1398-1401: "Running
in chroot, modules won't be signed") [S1]. `running_in_chroot()` (1198-1222)
ports Debian's `ischroot`. It returns true when `/proc/1/mountinfo` differs from
`/proc/self/mountinfo`, or when `/` differs from `/proc/1/root` by device and
inode. Anaconda's `%post` and the ISO's own `chroot "$root"` both trip it,
because PID 1 is the installer's systemd. A container whose PID 1 is its own
init does not trip it.

A sign-file failure only warns, and the module is installed unsigned (1657-1661).

Three more facts decide the ISO path:

- `dkms build` / `dkms install` with no `-k` target `uname -r` (413). Inside the
  installer that is the installer's kernel, not the target's. `xpadneo-dkms`'s
  `%post` runs exactly `dkms add`, `dkms build`, `dkms install` with no `-k`
  (`fedora/packages/rpm/xpadneo-dkms.spec`), so in the ISO it never builds for
  the target kernel. The installer kernel has no headers in the target, and
  `|| :` swallows the error.
- The kernel hook `40-dkms.install` runs `dkms kernel_postinst`, which is
  `autoinstall` (3234-3242), only for a kernel installed *after* the module was
  added. Anaconda installs the kernel before `provision` installs `dkms` and
  `xpadneo-dkms`, so the hook never fires for the target kernel.
- `autoinstall` skips any module already installed for that kernel (3059-3066).
  So `dkms.service` at boot (`dkms autoinstall --verbose --kernelver %v`) builds
  a module that was never built, but never re-signs one that was installed
  unsigned [S1].

The supported way to sign in the chroot is a drop-in in
`/etc/dkms/framework.conf.d/` with `try_sign_modules=true`. DKMS sources
`framework.conf` and then `framework.conf.d/*.conf` (975-980) [S1].

### akmods

- **Signing** happens inside `rpmbuild` through kmodtool's
  `%__spec_install_post`, which appends `%__kmodtool_modsign_install_post`
  (kmodtool 378-383). That macro calls `brp-kmodsign` only if signing is on
  (default 1) and *both* key paths exist (`-e`), defaulting to
  `/etc/pki/akmods/certs/public_key.der` and
  `/etc/pki/akmods/private/private_key.priv` (`macros.kmodtool` 39-52) [S3].
  There is no chroot check anywhere in akmods, akmodsbuild or kmodtool.
- **No key means an unsigned build, silently.** The `-e` test fails, the
  module is packaged unsigned, and `akmods` then treats that kmod as up to date:
  `check_kmod_up2date` compares only package versions (akmods 444-525). It is
  not rebuilt until the akmod or the kernel changes, or someone runs `--rebuild`.
- **The build runs as the `akmods` user.** It goes through `runuser ... akmods`
  (akmods 373). Through a symlink, a root-only `mok.key` passes the `-e` test,
  because `/var/lib/dkms` is `0755`. Then `sign-file` cannot open it and
  `brp-kmodsign` exits 1 ("modules are unsigned!", 39-43). rpmbuild runs build
  scripts with `/bin/sh -e`, so the build fails. akmods writes
  `<ver>-for-<kernel>.failed.log` and from then on prints "Ignoring nvidia-kmod
  as it failed earlier" (akmods 549-553). `akmods.service` at boot does not
  pass `--force`, so it never retries.
- **Install time is backgrounded.** `akmod-nvidia`'s `%posttrans` is
  `nohup /usr/sbin/akmods --from-akmod-posttrans --akmod nvidia &> /dev/null &`
  (generated by kmodtool 180-181; confirmed in the RPM) [S9]. akmods ignores the
  `--from-*posttrans` flag. With no `--kernels`, it builds for the grubby default
  kernel plus `uname -r`, keeping only kernels whose `kernel-devel` is
  installed (`check_default_kernel`, `check_kernel_devel`, akmods 273-324).
  `grubby` reads the target's boot entries inside the chroot, so the target
  kernel is picked, and the installer kernel is dropped for lack of headers.
  The job takes `flock` on `/run/akmods/akmods.lock` (268). It installs the
  built kmod with `dnf --disablerepo='*'` (406), and an install failure also
  leaves a failure marker.
- **Nothing waits for it.** The installer can finish and reboot while the
  backgrounded build is still compiling or installing.
- `/run/akmods` comes from `akmods.conf` in tmpfiles.d. systemd's file trigger
  creates it only when `/run/systemd/system` exists. Anaconda bind-mounts the
  installer's `/run` into the sysroot (`fsset.py` 149-158) [S5]. The installer
  should still create the directory itself (`install -d -m0770 -g akmods
  /run/akmods`) instead of relying on that.

## 3. `akmods-keygen@.service` and symlinked key files

The unit runs only if one of the two key paths is missing or empty:
`ConditionFileNotEmpty=|!/etc/pki/akmods/certs/public_key.der` and the same for
`private_key.priv` [S2]. systemd implements `ConditionFileNotEmpty` with
`stat()`, which follows symlinks, and checks for a regular file of non-zero
size (systemd v259 `condition.c` 1006-1016) [S10]. The unit runs as root.

| State of the two links | `akmods-keygen@` | `kmodgenca -a` |
|---|---|---|
| Both resolve to non-empty files (the #197 layout) | Skipped by the conditions | Would exit 0, "EXISTING KEY PAIR" (`readlink -e` on both, `kmodgenca` 335-346) |
| Both dangling (for example `mok.key` deleted) | Runs | Generates a new akmods-native pair, then `ln -nsf` *replaces* both links (540-549). The machine now has two keys and the akmods one is not enrolled |
| Exactly one dangling | Runs | Exits 3, "BROKEN SYMLINK(S) TO THE DEFAULT KEY PAIR" (302-321); akmods.service continues and builds unsigned |

`kmodgenca` skips the two canonical names when it scans for unpaired keys
(219-221, 246-248), so the links cause no warnings. Its own keys are created
under `umask 037` in group `akmods` and then `chmod g+r` (507, 529-530), so the
native mode is `root:akmods 0640`. The directories are `0750 root:akmods`
(`akmods.spec`) [S2]. A healthy link layout is therefore:

```
/etc/pki/akmods/private/private_key.priv -> /var/lib/dkms/mok.key   (link root:akmods; target root:akmods 0640)
/etc/pki/akmods/certs/public_key.der     -> /var/lib/dkms/mok.pub   (target 0644)
```

DKMS recreates *both* files when *either* is missing ("If any of the files ...
are non-existent, dkms will re-create both files", `framework.conf`). openssl
writes a fresh key as `0600`, so a doctor reconciler has to restore the
`akmods` group and the `0640` mode after any regeneration. The target is
labelled `var_lib_t`, which is fine: akmods runs unconfined
(`bin_t` binaries under `init_t`) [S6].

## 4. `mokutil` from the target root

- mokutil talks to firmware variables through libefivar. Before any command
  other than help, it calls `efi_variables_supported()` and exits 1 with "EFI
  variables are not supported on this system" when they are unavailable
  (mokutil.c 2159-2162) [S11].
- libefivar's efivarfs backend `statfs()`s `/sys/firmware/efi/efivars/`, or
  `$EFIVARFS_PATH`, and requires the efivarfs magic (efivar `efivarfs.c` 31-91)
  [S12]. So a chroot works exactly when efivarfs is mounted at that path inside
  it.
- Anaconda 44.30 and 45.30 add an `efivarfs` mount at
  `/sys/firmware/efi/efivars` to the target's system filesystems on any EFI
  platform, next to `/dev`, `/proc`, `/sys` and a bind of `/run`
  (`pyanaconda/modules/storage/devicetree/fsset.py` 90-169) [S5]. Upstream
  `main` additionally checks that the installer has efivarfs mounted first
  [S13]. The ISO's `stage` runs `chroot "$root"` inside those mounts, so
  `provision` sees the firmware variables.
- `--import` checks that `SecureBoot` is readable. It writes `MokNew` and
  `MokAuth` (mokutil.c 467-536) and skips a key already in `db`, `MokListRT` or
  `MokNew` (811-831). `--timeout` skips the Secure Boot check and writes
  `MokTimeout` (2166-2168, 1657-1680), and `-1` means wait for a keypress. These
  are firmware variables, so writing them from the target root and from the
  running system is the same operation.
- The password can be piped: `read_hidden_line` only touches the terminal when
  stdin is a TTY, then reads a line with `getline` (mokutil `util.c` 354-384)
  [S11]. So `printf 'enroll\nenroll\n' | mokutil --import /var/lib/dkms/mok.pub`
  works unattended, with the fixed password from #199.
- On a BIOS install there is no efivarfs and no Secure Boot. The installer
  should skip the queue (`mokutil --sb-state` fails) but still sign, so turning
  Secure Boot on later needs only an enrollment.

## 5. What the first boot looks like

With the key queued and the modules already built and signed in the target:

1. shim finds `MokNew` and starts MokManager, which waits indefinitely because
   of `MokTimeout=-1`. The user picks Enroll MOK, types `enroll`, and the
   machine reboots.
2. The kernel trusts the enrolled MOK for module signatures. xpadneo loads on
   demand through udev. The NVIDIA modules are not in the initramfs
   (`99-nvidia-dracut.conf` omits them) [S9], so they load from the root
   filesystem.
3. `dkms.service` and `akmods.service` find nothing to do.

If a build was deferred, it happens in that boot after enrollment:

- `dkms.service` (`Before=network-pre.target graphical.target`) runs
  `autoinstall` for the booted kernel. It builds xpadneo and signs it with the
  existing key, generating one only if none exists. A key generated here is
  *not* enrolled, so the module is rejected until a second enrollment.
- `akmods.service` (`Before=display-manager.service`) runs `akmods-keygen@`
  first (skipped when the links are valid), then `akmods --from-init` for
  `uname -r` only. It shows "akmod: Building ..." through Plymouth and builds
  NVIDIA, which takes minutes while the login screen waits. On success it
  modprobes all device modaliases and restarts `systemd-modules-load` (akmods
  580-588). It skips a kmod with a failure marker.
- `nvidia-fallback.service` (`After=akmods.service`,
  `Before=display-manager.service`) runs `modprobe nouveau` when the kernel
  command line carries `rd.driver.blacklist=nouveau,nova_core` and
  `/sys/module/nvidia` is absent. That covers a failed or skipped build, and a
  signature the kernel rejected because enrollment was skipped. RPM Fusion's
  package ships the unit but no preset for it [S9], so Ryoku has to enable it
  (#200). The blacklist arguments come from `xorg-x11-drv-nvidia`'s
  `%posttrans` `grubby` call, which also works in the chroot.
- If the user skips MokManager, nothing enrolled matches the signatures. NVIDIA
  falls back to nouveau, xpadneo does not load, and the queue has to be redone
  (the doctor re-queue from #199).

## 6. What each install path has to do

Let `K` be the target's default kernel (`grubby --default-kernel`, or the
newest `kernel-core`). The steps are the same on both paths. Only `K` and the
chroot policy differ.

### Fedora ISO (`fedora/iso/post/provision`, inside `chroot` of the sysroot)

The order matters, because `akmod-nvidia`'s install-time build has to find a
readable key:

1. **Before the desktop plan**, install `dkms` (and, on NVIDIA machines with
   RPM Fusion enabled, `akmods`). Installing `akmods` creates the `akmods` user
   and group through sysusers, and `/etc/pki/akmods/{certs,private}`.
2. **Create the key:** `dkms generate_mok`. It has no chroot check.
3. **NVIDIA only:** `chgrp akmods /var/lib/dkms/mok.key`, `chmod 0640` it,
   `ln -sfn` both akmods paths to the DKMS files, and `chown -h root:akmods` the
   links.
4. **Let DKMS sign in the chroot:** a `framework.conf.d` drop-in with
   `try_sign_modules=true`. It can be temporary in `provision`, or shipped,
   which also makes rescue-chroot rebuilds sign.
5. **Install the plan** (`xpadneo-dkms`, `kernel-devel` matching `K`,
   `akmod-nvidia`, `xorg-x11-drv-nvidia`, ...). The xpadneo `%post` build fails
   harmlessly. The akmod's backgrounded build now signs.
6. **Build for `K` in the foreground:** `dkms autoinstall -k "$K"`. Wait until
   no `akmods --from-akmod-posttrans` process remains, make sure `/run/akmods`
   exists, then run `akmods --force --kernels "$K"`. `--force` clears a
   failure marker from a raced background run.
7. **Verify** with `modinfo -k "$K" -F signer` for `hid_xpadneo` and `nvidia`.
   Both must report `DKMS module signing key`. If NVIDIA is unsigned, run
   `akmods --rebuild --force --kernels "$K"`.
8. **Queue the enrollment** when `mokutil --sb-state` reports Secure Boot on:
   `mokutil --timeout -1`, then pipe `enroll` twice into
   `mokutil --import /var/lib/dkms/mok.pub`. The import itself skips a key that
   is already enrolled or queued.
9. Enable `nvidia-fallback.service`. `dkms.service` and `akmods.service` are
   already enabled by the Fedora preset.

None of this needs the running system. Anaconda's mounts provide `/proc`,
`/sys`, efivarfs and `/run`, and `grubby` sees the target's boot entries.

### `ryoku-shell-installer` (running system, no chroot)

Today on Fedora, `defaultPlan` forces `nvidia=false` and `stepDrivers` only
prints that NVIDIA stays on nouveau (`engine.go` on `fedora/iso`, 275-279 and
2150-2156). xpadneo arrives through the package plan (`xpadneo-dkms` and `dkms`
in the desktop lane, `fedora/packages/translations.tsv`). Its enrollment is the
manual step in `docs/installation-hardware.md` (295-298).

- DKMS signs by default here, and the xpadneo `%post` build creates the key,
  but only when `uname -r` has matching headers. After the installer's own
  `dnf upgrade` the booted kernel is often older than the default one. Then
  xpadneo builds first in `dkms.service` on the next boot, which creates a key
  nobody queued. So the installer must not rely on the `%post` build. It needs
  the same explicit steps:
  `dkms generate_mok` (idempotent), then, NVIDIA only, `akmods` first, the
  group, mode and links, then RPM Fusion and `akmod-nvidia`. After that, run
  `dkms autoinstall -k "$K"` and `akmods --force` in the foreground. Without
  `--kernels`, `akmods --force` builds for the default and the running kernel.
  Then verify the signer and queue the enrollment as on the ISO, all before the
  reboot prompt.
- No chroot drop-in is needed. Everything runs through `sudo` on the live
  system, and the password goes in through stdin.

## Findings that change the plan

1. **The key cannot be root-only on NVIDIA machines.** akmods signs as the
   `akmods` user, so `mok.key` has to be `root:akmods 0640` (#197 item 6 says
   `0600`). A `0600` key makes NVIDIA builds fail outright and stick, because
   the failure marker blocks boot-time retries. Doctor must also heal the
   group and mode after DKMS regenerates the key.
2. **The key and links must exist before `akmod-nvidia` is installed.**
   Otherwise its install-time build is silently unsigned, and akmods considers
   it current until the next kernel.
3. **The ISO must build xpadneo for the target kernel itself.** Neither the
   package `%post` nor the kernel hook builds it there. Signing in the chroot
   needs `try_sign_modules=true`.
4. **The ISO must wait for the backgrounded akmods job** and then run akmods
   synchronously with `--force`. Otherwise the reboot can cut a build or a kmod
   install short.
5. **mokutil in the target root is supported** as long as Anaconda's efivarfs
   mount is present (UEFI installs). No step has to wait for the running system.

## Not verified here

These follow from the sources but have not yet been run in a VM: a full NVIDIA
build in Anaconda's chroot (`runuser` and PAM inside the chroot, and how long it
takes), and the OVMF Secure Boot MokManager flow with the queued DKMS key.
`fedora/iso/tests/vm-install.sh` has no NVIDIA GPU, so the akmod path can only
be smoke-tested there by checking that the key, the links and `MokNew` are in
place.

## Sources

- [S1] Fedora `dkms-3.4.4-1.fc44` / `dkms-3.4.3-2.fc45`: `/usr/bin/dkms`, `/etc/dkms/framework.conf`, `dkms.service`, `40-dkms.install`. Upstream (moved from dell/dkms), same line numbers: https://github.com/dkms-project/dkms/blob/v3.4.4/dkms.in
- [S2] Fedora `akmods-0.6.2-14.fc44` / `.fc45`: `/usr/sbin/akmods`, `akmods-keygen@.service`, `akmods-keygen.target`, `akmods.service`, `akmods@.service`, `kmodgenca` (`akmods-kmodgenca` in the SRPM), `akmods.spec`, `tmpfiles.d/akmods.conf`, `sysusers.d/akmods.conf`. Dist-git: https://src.fedoraproject.org/rpms/akmods (the `f44` `akmods` script matches the RPM byte for byte)
- [S3] Fedora `kmodtool-1.2.1-1.fc44` / `-2.fc45`: `/usr/bin/kmodtool`, `macros.kmodtool`, `brp-kmodsign`. Dist-git: https://src.fedoraproject.org/rpms/kmodtool
- [S4] Fedora `mokutil-0.7.2-3.fc44` / `-7.fc45` (changelog: rebuilds and an OpenSSL 4.0 patch)
- [S5] Fedora `anaconda-core-44.30-2.fc44` / `45.30-1.fc45`: `pyanaconda/modules/storage/devicetree/fsset.py`, `get_system_filesystems()`
- [S6] Live Fedora 44 box: `ls -lZ /var/lib/dkms`, `matchpathcon`, `rpm --eval '%{___build_shell} %{___build_args}'` (gives `/bin/sh -e`), `rpm -q --filetriggers systemd`
- [S7] akmods `README.secureboot` (`/usr/share/doc/akmods/README.secureboot`)
- [S8] `fedora-release-common-44-18`: `/usr/lib/systemd/system-preset/90-default.preset` (`enable dkms.service`, `enable akmods.service`)
- [S9] RPM Fusion `akmod-nvidia-615.78.08-1.fc44` (`%posttrans`, requires) and `xorg-x11-drv-nvidia-615.78.08-1.fc44` (`nvidia-fallback.service`, `99-nvidia-dracut.conf`, `%posttrans`)
- [S10] systemd v259 `src/shared/condition.c`, `condition_test_file_not_empty`: https://github.com/systemd/systemd/blob/v259/src/shared/condition.c
- [S11] mokutil 0.7.2 source: https://github.com/lcp/mokutil/blob/0.7.2/src/mokutil.c and https://github.com/lcp/mokutil/blob/0.7.2/src/util.c
- [S12] efivar 39 (`efivar-libs-39-12.fc44`): https://github.com/rhboot/efivar/blob/39/src/efivarfs.c
- [S13] Anaconda `main`: https://github.com/rhinstaller/anaconda/blob/main/pyanaconda/modules/storage/devicetree/fsset.py
- This repository, branch `fedora/iso`: `fedora/iso/kickstart/ryoku.ks`, `fedora/iso/post/stage`, `fedora/iso/post/provision`, `fedora/packages/rpm/xpadneo-dkms.spec`, `fedora/packages/translations.tsv`, `ryoku-shell-installer/engine.go`, `docs/installation-hardware.md`
