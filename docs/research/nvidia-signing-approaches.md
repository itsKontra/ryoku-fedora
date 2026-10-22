# NVIDIA driver sources and Secure Boot signing on Fedora

Research for [itsKontra/ryoku-fedora#194](https://github.com/itsKontra/ryoku-fedora/issues/194),
part of the map [#193](https://github.com/itsKontra/ryoku-fedora/issues/193).
State as of 2026-10-22. This note lays out the trade-offs and makes no choice.

Question: for Turing and newer GPUs on Fedora 44 and 45, which ways of getting
the NVIDIA open kernel modules plus userspace exist, and how does each satisfy
Secure Boot?

Evidence comes from primary sources: package metadata and scripts pulled from
the live repositories, upstream spec files, vendor docs, and Fedora change
pages. A Fedora 44 machine (kernel `7.2.9-200.fc44`, Secure Boot on) running
the fork's `ryoku-nvidia` also served as a live test case. Numbered references
are listed at the end.

## Short answer

Fedora signs no NVIDIA kernel module and ships none [S1][S2]. Every route that
loads the NVIDIA driver under Secure Boot depends on a Machine Owner Key (MOK)
enrolled through shim's MokManager, and there are only two kinds of signer:

- **A key generated on each machine** (akmods or DKMS). The target needs a
  compiler and kernel headers, and it builds the module itself when a new
  kernel arrives. Used by RPM Fusion, negativo17, NVIDIA's own Fedora repo, and
  Fedora's GNOME Software flow.
- **One key owned by the distributor**, used to sign prebuilt modules in CI.
  The target needs no toolchain, but the distributor holds the key and must
  publish a module for every kernel Fedora ships. Used by the fork's
  `ryoku-nvidia` and by Universal Blue (which avoids the kernel race because it
  ships kernel and modules together in one image).

| | Who signs, with what key | Enrolment | Toolchain on target | New kernel arrives before its module | Userspace and pinning | Ryoku maintenance |
|---|---|---|---|---|---|---|
| **A. RPM Fusion `akmod-nvidia`** | akmods on the target; per-machine key from `kmodgenca -a` in `/etc/pki/akmods` | `mokutil --import` the per-machine cert; user-set password; MokManager on next boot | gcc, make, rpm-build, kmodtool, `kernel-devel-matched`, pahole, lspci | Kernel installs; akmods builds in the background and again at boot. A build failure means no module; `nvidia-fallback.service` loads nouveau | RPM Fusion `xorg-x11-drv-nvidia`, newest production branch (615.78.08 on F44 and F45); pin with `dnf versionlock` or a legacy `580xx` package | Low: enable repo, generate and queue the key, doctor heals. RPM Fusion patches for new kernels |
| **B. Ryoku-owned key (fork `6e59699cc`)** | GitHub Actions; one Ryoku MOK key held as a CI secret; COPR packages only | `mokutil --import` the shipped Ryoku cert with a fixed password (`ryoku`); same key on every machine | None (prebuilt, signed modules) | The kernel is held by `Conflicts: kernel-core > newest` until CI publishes a module for it | RPM Fusion userspace pinned to the exact module version (`Requires: xorg-x11-drv-nvidia = 3:VER`) | High: key custody and rotation, a 6-hourly CI that must follow every Fedora kernel, repo hosting, breakage triage |
| **C. negativo17 `fedora-nvidia`** | akmods (per-machine akmods key) or DKMS (per-machine `/var/lib/dkms/mok.key`) | Same `mokutil` flow as A, for whichever key the build used | gcc-c++, kernel-devel, plus akmods or dkms | Same as A (akmods) or DKMS rebuild at kernel install | negativo17 `nvidia-driver*` (different package names, conflicts with RPM Fusion); one driver version per release; open modules only since 590 | Low to medium: like A, but a single-maintainer third-party repo |
| **D. NVIDIA CUDA repo** | DKMS on the target; per-machine `/var/lib/dkms/mok.key` | `mokutil` flow; NVIDIA documents a GNOME Software path that creates the MOK and shows a numeric password | gcc, make, `kernel-devel-matched`, dkms | DKMS rebuilds through the kernel-install plugin; failure means no module | NVIDIA `nvidia-driver*` (conflicts with RPM Fusion); repo keeps several versions, so `dnf versionlock` works. Only `fedora44` exists today, no `fedora45` | Low per release, but Ryoku waits on NVIDIA to publish each new Fedora release |
| **E. Fedora itself** | Fedora's kernel key signs only in-tree drivers (nouveau). No signed NVIDIA module exists | None for nouveau | None | Not applicable (in-tree) | Mesa (nouveau, NVK); the proprietary userspace is not in Fedora | None, but this is nouveau, not the NVIDIA driver |

Fedora 45 had not been released on 2026-10-22: `releases/45` is still absent
from the Fedora and RPM Fusion mirrors, while `development/45` and RPM Fusion
`updates/45` exist [S3]. RPM Fusion (`akmod-nvidia-615.78.08-1.fc45`) and
negativo17 (`fedora-45`) already carry F45 builds; NVIDIA's CUDA repo stops at
`fedora44` [S3][S4].

## How Fedora enforces module signatures

Fedora 44's kernel `7.2.9-200.fc44` is built with `CONFIG_MODULE_SIG=y`,
`CONFIG_MODULE_SIG_ALL=y`, `CONFIG_LOCK_DOWN_IN_EFI_SECURE_BOOT=y`,
`CONFIG_INTEGRITY_MACHINE_KEYRING=y`, `CONFIG_LOAD_UEFI_KEYS=y`, and
`CONFIG_MODULE_SIG_FORCE` unset (from `/boot/config-7.2.9-200.fc44.x86_64`).
With Secure Boot on, lockdown is `[integrity]`, so only modules signed by a
trusted key load. Keys enrolled in MOK are trusted for module signatures. The
fork's module on the test machine reports `signer: Ryoku NVIDIA module signing`
and `license: Dual MIT/GPL`, and it loads with Secure Boot enabled.

Without an enrolled key, an unsigned module or one signed by an unknown key
fails to load. RPM Fusion warns that the result is a disabled driver and a blank
screen [S5].

## A. RPM Fusion `akmod-nvidia` with a per-machine akmods key

**Source.** `akmod-nvidia` carries a small `nvidia-kmod` SRPM that builds from
`xorg-x11-drv-nvidia-kmodsrc`, the module source unpacked from NVIDIA's `.run`
file. On Fedora, RPM Fusion builds only the akmod (`%global buildforkernels
akmod`). The `kmod-nvidia` package in the Fedora repos is an empty metapackage
that requires `akmod-nvidia`; it is not a prebuilt module (verified with `dnf
repoquery --requires kmod-nvidia` on F44) [S6].

**Open modules.** By default the spec runs `nvidia-kmod-noopen-checks` at build
time. It reads `lspci` and uses `kernel-open` unless a GPU on RPM Fusion's
"no open" PCI ID list is present, so a Turing or newer machine gets the open
modules automatically. `--with kmod_nvidia_open` forces them [S6]. The howto
calls the result "open (but pre-built)": the OS-agnostic core,
`nv-kernel.o_binary`, ships precompiled inside NVIDIA's tarball. The separate
`akmod-nvidia-open`, built from the full GitHub source, lives in the tainted
repo and is discouraged [S5]. On F44 it sits at 595.58.03 and has no update.

**Signing.** kmodtool's macros sign every module that akmods builds with
`/etc/pki/akmods/private/private_key.priv` and `/etc/pki/akmods/certs/public_key.der`,
through `brp-kmodsign` and the kernel's `scripts/sign-file`. Signing is on by
default and is skipped silently when the key is missing (`macros.kmodtool` in
`kmodtool-1.2.1-1.fc44`) [S7]. The key comes from `kmodgenca -a`. The
`akmods-keygen@.service` unit runs it when either key file is empty, pulled in
by `akmods.service` [S7]. RPM Fusion has supported this automatic signing since
Fedora 36. Because the private key is stored on the machine, RPM Fusion calls
an encrypted root filesystem a "mandatory requirement" [S8].

**Enrolment.** `mokutil --import /etc/pki/akmods/certs/public_key.der`, a
password chosen at import, then "Enroll MOK" in MokManager on the next boot.
MokManager's keyboard is QWERTY. A firmware update may require re-importing the
key [S8][S9]. Each machine has its own key, so there is nothing to revoke
centrally.

**Toolchain on the target.** `akmods-0.6.2-14.fc44` requires gcc, make,
rpm-build, kmodtool, openssl, libelf, and `kernel-devel-matched`. `akmod-nvidia`
adds `/usr/bin/pahole`, `/usr/bin/lspci`, and `xorg-x11-drv-nvidia-kmodsrc`
[S6][S7]. Packages install the toolchain on every NVIDIA machine. That conflicts
with cardinal rule 7's "the installed target has no build toolchain
assumptions"; whether a packaged dependency counts is for the decision to
settle.

**New kernel.** `kernel-devel-matched` pulls in headers for each new kernel.
akmods' `kernel-install` plugin, `95-akmodsposttrans.install`, starts
`akmods@<kver>.service` with `--no-block`. `akmods.service` runs again at boot,
`Before=display-manager.service`, and builds whatever is missing [S7]. RPM
Fusion asks users to wait up to five minutes after the transaction [S5]. If the
source does not compile against the new kernel (for example, `nvidia-kmod`
needed "Add patch for 6.19rc kernel" in February 2026 [S6]), that kernel boots
without the module. `nvidia-fallback.service` from `xorg-x11-drv-nvidia` then
loads nouveau and shows a Plymouth message. It acts only when the command line
carries `rd.driver.blacklist=nouveau,nova_core`. The kernel itself is never
blocked.

**Userspace.** RPM Fusion `xorg-x11-drv-nvidia` (epoch 3), which requires
`nvidia-kmod >= 3:VER`. RPM Fusion updates the akmod and the userspace together:
615.78.08 is in F44 updates and in F45 [S3][S6]. Pinning means `dnf versionlock`
on the driver set [S5] or choosing a legacy branch package (`580xx`, which
covers Maxwell and Pascal, out of scope here).

**Fedora's own blessing of this route.** The accepted Fedora 41 change "Nvidia
Driver Installation with Secure Boot Support" made GNOME Software install RPM
Fusion's driver and enrol "a key for the user to self-sign the drivers" through
mokutil, asking the user for a password. FESCo approved it +6, 0, -2; dissent
questioned blanket auto-signing of every akmod [S2][S10]. Ryoku's desktop does
not use GNOME Software, so Ryoku would script the same steps itself.

**Ryoku maintenance.** Enable RPM Fusion on NVIDIA machines, install
`akmod-nvidia`, run `kmodgenca -a` and `mokutil --import` before the first
reboot, and let doctor heal a skipped enrolment or an unsigned module. RPM
Fusion's maintainers carry the kernel compatibility patches. Ryoku holds no key
and runs no CI.

## B. Prebuilt modules signed with a Ryoku-owned key (fork commit `6e59699cc`)

**Source and build.** CI rebuilds the `nvidia-kmod` SRPM extracted from RPM
Fusion's signed `akmod-nvidia` with `--with kmod_nvidia_open`. It builds one
kernel at a time for the three newest kernels in Koji's `f44-updates` tag
(three matches the default `installonly_limit`). It verifies signer,
certificate serial, and the open license, then wraps the signed `.ko` files in
an SRPM that COPR packages (`release/rpm/nvidia/prepare-srpm.sh`,
`fetch-kernels.py`, `.github/workflows/publish-nvidia-kmod.yml` at
`6e59699cc`).

**Signing.** `_kmodtool_signmodules_privkey` and `_pubkey` point at the Ryoku
key, which is materialised from the GitHub secret `RYOKU_NVIDIA_MOK_KEY` in the
`fedora-publish` environment. COPR never holds the key. The public certificate
`release/rpm/nvidia/ryoku-mok.der` (CN "Ryoku NVIDIA module signing") is valid
until 2126.

**Enrolment.** `ryoku-nvidia install` queues the shipped certificate with
MokManager using the documented password `ryoku` and an indefinite MokManager
timeout. Doctor re-queues a skipped enrolment. Every Ryoku NVIDIA machine trusts
the same key.

**Toolchain on the target.** None. This is the only option that meets rule 7 to
the letter.

**New kernel.** `ryoku-nvidia` declares `Conflicts: kernel-core > <newest built
kernel>` and `Requires: ryoku-nvidia-kmod-<newest kernel>`, and each kmod
subpackage requires `kernel-uname-r = <kernel>`. dnf therefore holds a newer
kernel until CI publishes a matching module. That is the one documented
exception to Ryoku's "never block the kernel" rule. Security kernel updates wait
too.

**Userspace.** RPM Fusion `xorg-x11-drv-nvidia`, pinned exactly with `Requires:
xorg-x11-drv-nvidia = 3:VER`. A userspace update therefore also waits for a
matching CI build.

**Live findings on the maintainer's machine (2026-10-22).**

- Installed: `ryoku-nvidia-615.71.09-40`, with modules for `7.2.7-200` and
  `7.2.9-200`. Fedora `updates` offers `kernel-core-7.2.11-200.fc44`, built
  2026-10-15, and RPM Fusion offers userspace 615.78.08.
- `dnf upgrade kernel-core` reports that the installed `ryoku-nvidia` conflicts
  with `kernel-core > 7.2.9-200`. Every newer `ryoku-nvidia` on COPR (`-52`,
  `-55`, `-56`, `-58`) requires a kmod for `7.2.9-201` or `7.2.10-200`, and
  neither kernel is in Fedora's repos any more. Fedora's `updates` repo carries
  only the newest build of each package. The machine is stuck on driver
  615.71.09 and kernel 7.2.9 until a build for 7.2.11 is published.
- The publish workflow no longer exists on the fork's default branch, and
  scheduled workflows only run from there. Its last run was 2026-10-16. No
  module for 7.2.11 has been built.

These findings show two maintenance hazards for any prebuilt approach that
installs through dnf. Building from Koji tags rather than from the repo that
users actually see can target kernels the repo no longer carries. And the
kernel hold depends on a scheduler staying alive.

**Ryoku maintenance.** The open questions on map #193 list key custody (where
the private key lives, CI secrets, rotation, revocation). Rotating the key means
re-enrolling on every machine. A leaked key lets anyone sign a kernel module
that every Ryoku NVIDIA machine will load until it is deleted from each MOK
list (`mokutil --delete`). Beyond the key, CI has to follow every Fedora kernel
push for each supported release (F44 and F45 in parallel around a release),
someone has to triage RPM Fusion source breakage on new kernels, and the
modules have to be published, today through COPR rather than the `[ryoku]`
repo that rule 7 names.

**Prior art: Universal Blue.** Universal Blue builds akmods (including
`nvidia-open`) into daily OCI images signed with its own key. The
`ublue-os-akmods-addons` package "installs extra repos and our kmods signing
key" [S11]. Users enrol it with the password `universalblue`; `ujust
enroll-secure-boot-key` runs `mokutil --timeout -1` and imports
`/etc/pki/akmods/certs/akmods-ublue.der` [S12]. Because the kernel and the
modules ship together in one image, no window opens where a new kernel lacks
its module. A dnf-based Ryoku does not have that property.

## C. negativo17 `fedora-nvidia`

**Source.** `nvidia-kmod` builds from NVIDIA's `open-gpu-kernel-modules` GitHub
tarball (full source, so it needs `gcc-c++`), plus two patches from Anatase
added on 2026-09-22. The 590.44.01 release on 2025-12-05 dropped the
proprietary module: "Drop proprietary modules support (required only for
vGPU)". negativo17 is open modules only, which suits Turing and newer [S13].

**Packaging.** On Fedora it ships `akmod-nvidia` (`%define buildforkernels
akmod`) or `dkms-nvidia`. A binary `kmod-nvidia` is offered only for EL [S13][S14].
Builds for 615.78.08 exist for `fedora-44` and `fedora-45` [S3].

**Signing and enrolment.** The akmod path signs through kmodtool with the
per-machine akmods key (2023: "Drop custom signing and compressing in favour of
kmodtool") [S13]. The DKMS path signs with dkms' per-machine
`/var/lib/dkms/mok.key` and `mok.pub` [S15]. In both cases the user enrols that
key with mokutil, as in A. The negativo17 article itself still tells users to
disable Secure Boot or sign modules themselves [S14].

**New kernel.** Same as A for akmods, same as D for DKMS.

**Userspace.** negativo17's own `nvidia-driver`, `nvidia-driver-libs`, and
`nvidia-driver-cuda` packages. The names differ from RPM Fusion's, and the
article says to remove other NVIDIA repos' packages first. Each release carries
one driver version, the newest [S14].

**Ryoku maintenance.** Similar to A. Upstream is one maintainer (Simone
Caronni), which is a bus-factor risk. Updates have been prompt (615.78.08 landed
2026-10-07).

## D. NVIDIA's own CUDA repository

**Packages.** `cuda/repos/fedora44/x86_64` publishes `nvidia-open`, which
requires `kmod-nvidia-open-dkms` or `kmod-nvidia-open`, plus the
`nvidia-driver*` userspace. For Fedora only the DKMS module is published
(`kmod-nvidia-open-dkms-615.71.10`), and nothing precompiled [S4][S16]. The
install guide covers only Fedora 44 and asks for `kernel-devel-matched
kernel-headers` first [S16]. As of 2026-10-22 there is no `fedora45`
directory [S4].

**Signing and enrolment.** DKMS signs with its per-machine key (Fedora's
`dkms-3.4.4` defaults to `mok_signing_key=/var/lib/dkms/mok.key`;
`try_sign_modules=not_in_chroot` means it does not sign inside an installer
chroot by default) [S15]. NVIDIA's GNOME Software integration page, which lists
Fedora 44, has GNOME Software create the MOK, show a password to note down, and
offer "Restart & Enroll MOK" [S17].

**Toolchain on the target.** gcc, make, `kernel-devel-matched`, dkms [S15][S16].

**New kernel.** DKMS hooks `kernel-install` (`40-dkms.install`) and rebuilds for
the new kernel. A failed build leaves that kernel without the driver.

**Userspace and pinning.** NVIDIA's packages keep several versions in the repo,
and the guide documents `dnf versionlock add` for DNF 5 [S18]. They conflict
with RPM Fusion's; NVIDIA's GNOME Software page has users pick the "CUDA
repository" because RPM Fusion also ships the driver [S17]. The version lags
slightly (615.71.10 against RPM Fusion's 615.78.08).

**Contrast with RHEL.** On RHEL 9, Red Hat compiles and signs the open modules,
so they load without extra key enrolment [S19]. Nothing equivalent exists for
Fedora.

## E. What Fedora itself ships

- **No NVIDIA kernel module.** On F44, `fedora` and `updates` carry only
  `nvidia-gpu-firmware`, `libva-nvidia-driver`, and unrelated tools (`dnf
  repoquery` on 2026-10-22). Fedora has not permitted add-on kernel modules
  since Fedora 8 [S1]. The F41 change page notes that the driver must come from
  RPM Fusion because of NVIDIA's licence [S2].
- **No Fedora-signed third-party kmods.** Fedora's only NVIDIA Secure Boot
  feature is the F41 GNOME Software flow, which self-signs with a per-machine
  akmods key (option A).
- **nouveau** is in-tree and signed by Fedora's kernel key, so it loads under
  Secure Boot with no enrolment and no toolchain. On Turing and newer it runs
  on GSP firmware from `nvidia-gpu-firmware`. It is the out-of-scope fallback
  the map already keeps for older GPUs, and it is not the NVIDIA driver.
- **Nova.** Fedora 44's `7.2.9` kernel builds neither `CONFIG_NOVA_CORE` nor
  `CONFIG_DRM_NOVA`. Upstream work in 7.2 is still enablement: GSP boot,
  Hopper and Blackwell, GA100 [S20]. Nova is not a desktop option. RPM Fusion
  already adds `nova_core` to the blacklisted modules for OSTree [S5].
- **Fedora 45.** A search found no F45 change touching NVIDIA kernel modules or
  signing (low confidence: the official ChangeSet and schedule pages were
  behind a bot wall during this research). GNOME 51's Mutter in F45 is
  reported to drop its legacy NVIDIA interfaces, which is userspace and does
  not affect signing.

## Cross-cutting points for the decision

1. **Signer custody versus target toolchain is the core trade.** A, C, and D
   keep the key on each machine and need a compiler there. B removes the
   compiler but makes Ryoku a signing authority that every NVIDIA machine
   trusts at the kernel level.
2. **Kernel-ahead-of-module behaviour.** A, C, and D never block the kernel;
   at worst a kernel boots on nouveau until the module builds or is fixed. B
   blocks the kernel, and its userspace, until CI catches up. The live
   machine shows how that fails when CI stops or targets kernels the repo no
   longer carries.
3. **Installer timing.** akmods normally builds at boot, before the display
   manager starts. DKMS skips signing inside a chroot by default. Every
   per-machine-key option therefore has to settle where the key is generated
   and queued: in the target chroot before the first reboot, so the first
   MokManager visit enrols it. B needs only the `mokutil --import` of a file
   it ships.
4. **Repo mixing.** RPM Fusion, negativo17, and NVIDIA's CUDA repo use
   conflicting package names for the same driver. Whichever source is chosen
   should be the only NVIDIA source enabled on the machine.
5. **Fedora 45 coverage.** RPM Fusion and negativo17 already publish F45
   builds. NVIDIA's CUDA repo does not yet. B would need a second Koji tag and
   a second kernel set.
6. **Turing and newer only.** Every option above serves open modules on
   Turing and newer: RPM Fusion by auto-detection or a flag, negativo17 by
   being open-only, NVIDIA through `nvidia-open`, and the fork through
   `--with kmod_nvidia_open`.

## Sources

- [S1] Fedora wiki, "Packaging Kernel-modules for Fedora" (add-on modules not permitted after Fedora 8): https://fedoraproject.org/wiki/Extras/Policy/KernelModules
- [S2] Fedora change "Nvidia Driver Installation with Secure Boot Support" (F41, accepted): https://fedoraproject.org/wiki/Changes/NvidiaInstallationWithSecureboot
- [S3] Repository indexes checked 2026-10-22: https://dl.fedoraproject.org/pub/fedora/linux/development/45/ , https://download1.rpmfusion.org/nonfree/fedora/updates/45/x86_64/a/ , https://download1.rpmfusion.org/nonfree/fedora/development/45/ , https://negativo17.org/repos/nvidia/fedora-45/x86_64/ ; F44 versions via `dnf repoquery` against `fedora`, `updates`, and RPM Fusion nonfree
- [S4] NVIDIA CUDA repo index: https://developer.download.nvidia.com/compute/cuda/repos/ and https://developer.download.nvidia.com/compute/cuda/repos/fedora44/x86_64/ ; `rpm -qp --requires nvidia-open-615.71.10-1.fc44.noarch.rpm`
- [S5] RPM Fusion Howto/NVIDIA (last edited 2026-10-05): https://rpmfusion.org/Howto/NVIDIA
- [S6] RPM Fusion `akmod-nvidia-615.78.08-1.fc44`: embedded `nvidia-kmod.spec`, `nvidia-kmod-noopen-checks`, and changelog; package requires; `kmod-nvidia` requires (downloaded with `dnf download`). Git: https://github.com/rpmfusion/nvidia-kmod
- [S7] Fedora `akmods-0.6.2-14.fc44` (`README.secureboot`, `akmods-keygen@.service`, `akmods.service`, `95-akmodsposttrans.install`, requires) and `kmodtool-1.2.1-1.fc44` (`macros.kmodtool`, `brp-kmodsign`)
- [S8] RPM Fusion Howto/Secure Boot (last edited 2025-11-06): https://rpmfusion.org/Howto/Secure%20Boot
- [S9] akmods `README.secureboot`, shipped at `/usr/share/doc/akmods/README.secureboot`
- [S10] FESCo ticket #3238: https://pagure.io/fesco/issue/3238
- [S11] Universal Blue akmods: https://github.com/ublue-os/akmods
- [S12] Universal Blue forum, Secure Boot key enrolment: https://universal-blue.discourse.group/t/secure-boot-notice/405 and https://universal-blue.discourse.group/t/secure-boot-key-mok-management/4310
- [S13] negativo17 `nvidia-kmod` spec and history: https://github.com/negativo17/nvidia-kmod
- [S14] negativo17 "Nvidia driver" article: https://negativo17.org/nvidia-driver/ ; repo file https://negativo17.org/repos/fedora-nvidia.repo ; `dkms-nvidia` spec https://github.com/negativo17/dkms-nvidia
- [S15] Fedora `dkms-3.4.4-1.fc44`: `/etc/dkms/framework.conf`, `40-dkms.install`, requires
- [S16] NVIDIA Driver Installation Guide, Fedora: https://docs.nvidia.com/datacenter/tesla/driver-installation-guide/fedora.html
- [S17] NVIDIA Driver Installation Guide, GNOME Software integration: https://docs.nvidia.com/datacenter/tesla/driver-installation-guide/gnome-software-integration.html
- [S18] NVIDIA Driver Installation Guide, version locking: https://docs.nvidia.com/datacenter/tesla/driver-installation-guide/version-locking.html
- [S19] NVIDIA blog, open GPU datacenter drivers for RHEL 9 signed by Red Hat (2025-02-10): https://developer.nvidia.com/blog/nvidia-open-gpu-datacenter-drivers-for-rhel9-signed-by-red-hat
- [S20] Phoronix, Nova in Linux 7.2: https://www.phoronix.com/news/Linux-7.2-DRM-Rust
- Fork implementation: commit `6e59699cc` in this repository (`release/rpm/nvidia/`, `system/hardware/gpu/ryoku-nvidia`, `ryoku/cli/internal/doctor/reconcile_nvidia_signed.go`)
