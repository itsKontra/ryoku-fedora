# The fork's signed `ryoku-nvidia` in practice

Research for itsKontra/ryoku-fedora#195 (map: #193, feeds #197). Date: 2026-10-22.

Question: what did the fork's `ryoku-nvidia` do end to end, how did it behave on
the maintainer's Fedora 44 machine, and which parts are worth carrying into an
upstream-bound version? This note does not pick the final approach.

## Sources

- Commit `6e59699cc` ("[release] Secure Boot signed NVIDIA driver for Turing and
  newer GPUs (#31)", 2026-09-26). It is reachable from `fork/fedora-backup-alpha`
  and a few port branches, not from the fork's current `main`. The fork's `main`
  now points at upstream `98a036825`, and `release/rpm/nvidia` returns 404 there.
  No later commit on any branch touched `release/rpm/nvidia`,
  `.github/workflows/publish-nvidia-kmod.yml` or `system/hardware/gpu/ryoku-nvidia`
  (`git log --all` over those paths returns only `6e59699cc`).
- GitHub Actions history of `publish-nvidia-kmod.yml` on itsKontra/ryoku-fedora
  (`gh run list`, `gh run view --log-failed`). The workflow's state is `deleted`
  since 2026-10-16 22:00 UTC.
- COPR API: `api_3/build/list` and `api_3/project` for `itskontra/ryoku-nvidia`.
- Bodhi API: F44 kernel updates (`date_stable` per build).
- The host itself, read-only and without root: `rpm`, `dnf repoquery`,
  `dnf history`, `dnf upgrade --assumeno`, `mokutil`, `modinfo`,
  `/proc/driver/nvidia/version`, `journalctl` across 28 boots.
- `roadmap/tickets/FP-001.md` and `open-issues.md` in the main checkout.

## What the fork built, end to end

1. **Build** (`.github/workflows/publish-nvidia-kmod.yml`, cron every 6 h plus
   manual dispatch, `fedora:44` container, only on `itsKontra/ryoku-fedora` `main`).
   `prepare-srpm.sh`:
   - imports the pinned Fedora 44 and RPM Fusion keys, asks Koji
     (`listTagged f44-updates kernel`) for the three newest kernels and downloads
     their signed `kernel-devel` (`fetch-kernels.py`, checks `rpmkeys --checksig`);
   - reads the newest `xorg-x11-drv-nvidia` version in RPM Fusion, and exits early
     if the COPR repo already has `ryoku-nvidia-kmod-<newest kernel>` at that version;
   - unpacks the RPM Fusion-signed `akmod-nvidia` to get its `nvidia-kmod` SRPM,
     rebuilds it `--with kmod_nvidia_open` once per kernel, with kmodtool's signing
     macros pointed at the Ryoku private key (from the `RYOKU_NVIDIA_MOK_KEY`
     environment secret, written to a 0600 temp file);
   - `verify-modules.sh` checks every `.ko` names the Ryoku signer and the
     certificate serial, that there are at least 4 modules, and that `nvidia.ko`
     reports `Dual MIT/GPL` (the open module);
   - wraps the already-signed modules into the `ryoku-nvidia` SRPM. COPR never
     sees the key; `__os_install_post` is disabled so nothing strips the signature.
2. **Test** (keyless job): `rpmbuild --rebuild` of the SRPM, then
   `test-install.sh` installs it with `tsflags=noscripts` next to the RPM Fusion
   userspace, checks the `Conflicts: kernel-core > <newest>` line and re-verifies
   the signatures on disk.
3. **Publish**: `copr-build.py` submits the SRPM to `itskontra/ryoku-nvidia`,
   `verify-rpms.py` checks the COPR output against the pinned COPR key, and
   `publish.py` (extended with a `TARGETS` allowlist) regenerates the repo.
4. **Package shape** (`ryoku-nvidia.spec`): one `ryoku-nvidia-kmod-<kver>`
   subpackage per kernel (`Requires: kernel-uname-r = <kver>`), the main package
   `Requires` only the newest kernel's kmod, pins `xorg-x11-drv-nvidia = 3:<driver>`,
   `Conflicts: kernel-core > <newest>`, conflicts with `akmod-nvidia`,
   `akmod-nvidia-open`, `kmod-nvidia`, ships the DER cert to
   `/usr/share/ryoku/keys/ryoku-mok.der` and a preset enabling RPM Fusion's
   `nvidia-fallback.service`.
5. **Host tool** (`system/hardware/gpu/ryoku-nvidia`, bash):
   `status` prints `gpu= driver= secureboot= key=` facts; `install` acts only on
   PCI device >= `0x1e00` (Turing+), leaves a host `akmod-nvidia` alone, enables the
   COPR, installs, then `enroll`; `enroll` runs `mokutil --import` with the fixed
   password `ryoku` and `mokutil --timeout -1`, and does nothing when Secure Boot is off.
6. **Installer hooks**: the ISO provisioner (`provision-target.py`) runs
   `ryoku-nvidia install` in the target chroot after the vendor scripts (mounting
   efivarfs first); the shell installer's driver step runs it from the payload.
7. **Doctor** (`reconcile_nvidia_signed.go`): reads `ryoku-nvidia status`,
   re-queues a missing key under Secure Boot, warns on a pending one, suggests
   `sudo ryoku-nvidia install` for a Turing+ GPU on nouveau, never replaces a
   host akmod driver.
8. **Key**: RSA-4096 self-signed cert `O=Ryoku, CN=Ryoku NVIDIA module signing`,
   valid to 2126, EKU limited to module signing (`1.3.6.1.4.1.2312.16.1.2`).
   `generate-mok-key.sh` refuses to overwrite it because rotation strands every
   enrolled machine.

Note on wording: `open-issues.md` calls the host's driver "proprietary 615.71.09".
The kernel module is NVIDIA's **open** module (`modinfo nvidia`: `license: Dual MIT/GPL`,
`/proc/driver/nvidia/version`: "NVIDIA UNIX Open Kernel Module ... 615.71.09");
only the userspace (`xorg-x11-drv-nvidia*` from RPM Fusion) is proprietary.

## What the host shows

Host: RTX 2060 (TU106, `10de:1f08`), Fedora 44, Secure Boot **enabled**, booted
`7.2.9-200.fc44`.

| Fact | Evidence |
|---|---|
| Signed open module loads under Secure Boot + lockdown | `modinfo nvidia`: `signer: Ryoku NVIDIA module signing`, `sig_key 62:19:13:E9...`; kernel log "Kernel is locked down from EFI Secure Boot mode" and "Loaded X.509 cert 'Ryoku NVIDIA module signing'" every boot since 2026-09-28 |
| Key enrolled | `mokutil --list-enrolled` lists the Ryoku cert; `mokutil --test-key /usr/share/ryoku/keys/ryoku-mok.der`: "already enrolled" |
| Installed | `ryoku-nvidia-615.71.09-40`, `ryoku-nvidia-kmod-7.2.7-200...-40`, `ryoku-nvidia-kmod-7.2.9-200...-40`; RPM Fusion `xorg-x11-drv-nvidia*-615.71.09-3` |
| kmod and userspace versions agree | both 615.71.09; the `=` pin held them together |
| No akmods, no compiler path | no `akmod-*`; only `kernel-devel` from unrelated dev work |
| Fallback | `nvidia-fallback.service` enabled by the preset |

### Timeline on the host

- 2026-09-27 21:39 local: `sudo ryoku-nvidia install` run **by hand** (the ISO
  provisioner path was not exercised here; the system was installed earlier that
  day). Secure Boot was **off**, so `enroll` queued nothing. The module loaded with
  "module verification failed: signature and/or required key missing - tainting kernel".
- 23:53, 23:57, 00:00: three boots with Secure Boot **on**. Each logged "Loading of
  module with unavailable key is rejected"; `nvidia-fallback.service` ran and the
  desktop came up on nouveau. The user ran `sudo ryoku-nvidia enroll` from tty1,
  rebooted, approved in MokManager.
- 2026-09-28 00:04: first boot with the Ryoku cert loaded; NVIDIA loaded. Every
  Secure Boot boot since (20+) loaded it without issue.
- 2026-10-10: `dnf -y upgrade` installed kernel `7.2.9-200` together with its kmod
  and moved the main package from `-1` to `-40` in one transaction. No hold was
  visible because the module was already published (release 40, 2026-10-08).
- Since then no full `dnf upgrade` ran (2026-10-19 was a `distro-sync` of named
  Ryoku packages), so the host never displayed a hold until now.

### State today: frozen, and the hold leaks

`dnf upgrade --assumeno` (no root needed to resolve) on the host:

- Skips `kernel-core-7.2.11` ("Skipping packages with conflicts") and every
  `ryoku-nvidia` build, because the newest COPR build (58, 2026-10-14) requires
  `ryoku-nvidia-kmod-7.2.10-200`, which requires `kernel-uname-r = 7.2.10-200`, and
  Fedora's `updates` repo now carries only `7.2.11`. No repo provides 7.2.10 any more
  (`dnf repoquery --whatprovides 'kernel-uname-r = 7.2.10-200.fc44.x86_64'` is empty).
  Older builds need 7.2.8 / 7.2.9-201, also gone.
- Silently keeps the RPM Fusion userspace at 615.71.09 although 615.78.08 is out
  (`dnf check-upgrade` lists it; the transaction does not, and nothing says why).
  The old `615.71.09-3` userspace is no longer in RPM Fusion either.
- **Still installs 7.2.11 `kernel-modules`, `kernel-modules-core`, `kernel-devel`
  and `kernel-uki-virt` (plus weak dep `uki-direct`)** while skipping `kernel-core`.
  `kernel-modules-core-7.2.11` requires `kernel-uname-r = 7.2.11`, which both
  `kernel-core` and `kernel-uki-virt` provide; with `kernel-core` blocked, the
  solver satisfies it with the UKI. The `Conflicts: kernel-core > X` hold does not
  cover the other providers of `kernel-uname-r`. Whether that UKI then becomes a
  boot entry on this GRUB host was not tested (needs root); at minimum it puts a
  ~90 MiB kernel image without an NVIDIA module into `/boot/efi/EFI/Linux`.

The cause of the freeze is that the build stopped: the fork's `main` was reset
to upstream and GitHub marked the workflow deleted at 2026-10-16 22:00 UTC; kernel
7.2.11 went stable three hours later. Nothing alerts the user; the only symptom is
a kernel that never arrives.

## How the pipeline behaved (68 runs, 2026-09-25 to 2026-10-16)

COPR: 7 builds, **all succeeded** (releases 1, 19, 40, 52, 55, 56, 58). Every
other run either exited early ("already published") or failed before COPR.

GitHub Actions: 59 success, **9 failures, all external infrastructure, none in
Ryoku code**:

| Runs | Step | Cause |
|---|---|---|
| 22, 43, 63 (build), 57 (test) | `enable-rpmfusion.sh` | one-shot `curl` to RPM Fusion mirrors: HTTP 500, DNS failure for a mirror host, connect timeouts |
| 21 (build) | `fetch-kernels.py` | Koji XML-RPC `Fault 1014: 'database outage'` |
| 46 (build) | `fetch-kernels.py` | `Network is unreachable` to Koji |
| 49, 50, 51 (test) | `test-install.sh` | `nothing provides kernel-uname-r = 7.2.9-201.fc44.x86_64`: Koji had tagged the kernel into `f44-updates` about a day before Bodhi marked it stable and mirrors carried it |

Neither download path retries. Runs 49 to 51 built and signed modules that were
thrown away; the keyless test job did its job there (publishing would have made
`ryoku-nvidia` uninstallable until mirrors caught up), but it surfaced as three
red runs. Release 56 is an identical rebuild of 55 (same driver, same three
kernels): the "already published" probe swallows errors (`|| true`) and rebuilds.

### Kernel hold windows (Bodhi `date_stable` to COPR build end)

| Kernel | Stable (UTC) | Module published (UTC) | Hold |
|---|---|---|---|
| 7.2.8-200 | 10-01 01:11 | 10-01 06:21 (rel 19) | ~5 h |
| 7.2.9-200 | 10-08 01:15 | 10-08 06:30 (rel 40) | ~5 h |
| 7.2.9-201 | 10-12 01:01 | 10-12 06:21 (rel 52) | ~5 h (attempts started ~19 h earlier, see above) |
| 7.2.10-200 | 10-14 01:20 | 10-14 12:22 (rel 58) | ~11 h (run 57 lost to a mirror timeout) |
| 7.2.11-200 | 10-17 01:03 | never | indefinite (workflow gone) |

While the pipeline ran, holds were short: Fedora's stable pushes land around
01:00 UTC and the 06:00 cron run picked them up. Mirrors lag a few hours behind
Bodhi, so most users likely saw little or no hold (inferred, not measured). The driver bump
615.71.09 to 615.78.08 (RPM Fusion) was picked up the same day (rel 55, 10-13).

## What worked

- **Signing outside COPR.** The key never reaches COPR; COPR only repackages
  pre-signed modules. Signature, serial and open-license checks gate every build.
  On real hardware the result loads under Secure Boot lockdown, every boot.
- **Reusing RPM Fusion's own `nvidia-kmod` source and kmodtool signing macros**
  rather than a custom build. Seven builds across two driver versions and five
  kernels with zero build-logic failures.
- **The kmod/userspace pin.** `xorg-x11-drv-nvidia = 3:<driver>` kept module and
  userspace on the same version on the host the whole time.
- **The kernel hold, as long as builds flowed.** Short windows, no boot without a
  module on this host.
- **`nvidia-fallback.service`.** Three Secure Boot boots without the key landed on
  a working nouveau desktop, not a black screen.
- **`ryoku-nvidia status` as the single fact source** for doctor, and doctor's
  "never replace a host akmod driver" stance.
- **Keyless test job before publication**: it caught the Koji-ahead-of-mirrors case.
- No compiler on user machines, no akmods rebuild at boot.

## What broke or caused friction

1. **Single point of failure, no alarm.** When the workflow stopped, every
   installed machine silently froze on its kernel and driver, and the newest
   published package became uninstallable. Nothing (doctor, update lane, COPR)
   reports "your kernel is held because the module pipeline is stale".
2. **Builds target kernels that disappear.** The main package requires the kmod of
   the newest *built* kernel, and Fedora's `updates` keeps only the newest kernel,
   so any lag or stop leaves `ryoku-nvidia` resolvable only on machines that
   already have that exact kernel. The same applies to the `=` userspace pin once
   RPM Fusion moves on (RPM Fusion keeps only the newest in `updates`). A fresh
   install during such a window fails; the tool then leaves nouveau.
3. **The hold leaks** through `kernel-uki-virt` (and pulls `kernel-modules*` of the
   held version): `Conflicts: kernel-core > X` is not a complete kernel hold.
4. **Silent holds.** dnf5 lists "Skipping packages with conflicts" for
   `kernel-core` and drops the NVIDIA userspace upgrade without a word. The fork's
   docs promise the distribution lane "waits", but the user gets no reason.
5. **Enrolment UX when Secure Boot is off at install time.** `enroll` skips
   queuing when Secure Boot is off. Turning Secure Boot on later rejected the
   module on three boots until the user ran `ryoku-nvidia enroll` by hand. Doctor
   would have re-queued it, but only if run, and still costs a MokManager round.
   Queuing the key regardless of the current Secure Boot state would remove this.
6. **Shared fixed MOK password `ryoku`** and US-QWERTY MokManager: workable and
   documented, but it is a public password on a key every Ryoku box trusts.
7. **Key custody.** The private key lives in a GitHub environment secret of a
   personal fork; a 100-year cert that cannot be rotated without re-enrolling every
   machine. Moving the pipeline upstream means either taking over that key (and
   its trust in every enrolled fork machine) or minting a new one.
8. **Flaky external fetches.** 9/68 runs failed on mirror, DNS or Koji outages;
   neither `curl` nor the Koji client retries. Harmless while the 6 h cron retried,
   but it inflates red CI and hold windows (7.2.10: 11 h).
9. **Koji tag vs mirror lag.** Picking kernels from the Koji tag means building
   for kernels users cannot install yet; the test job then fails until mirrors
   catch up. Querying the mirror metadata (`dnf repoquery` on `updates`) instead
   would match what users actually get.
10. **Old kernels keep old modules (design risk, not observed).** A build carries
    only the three newest kernels. After a driver bump, the kmod for an older
    installed kernel (here 7.2.7, still at `615.71.09-40`) is not rebuilt, while the
    pin moves userspace forward. Booting that fallback kernel would then pair a
    615.71.09 module with 615.78.08 userspace. The host never reached this state
    because it never took the 615.78.08 build.
11. **Fork coupling.** `itskontra/ryoku-nvidia` is hard-coded in the tool and
    `publish.py`; the COPR description still links to a `main` path that is gone.
12. **Single release, single arch.** `fedora-44-x86_64` and `f44-updates` are
    hard-coded; F45 would need parallel builds.

## Reuse vs rewrite (input for #197, not a decision)

| Part | Verdict | Why |
|---|---|---|
| `verify-modules.sh` | reuse as is | small, precise, catches wrong key/serial/closed module |
| `generate-mok-key.sh` (EKU-limited, refuse-overwrite) | reuse the approach | the right cert profile; key custody decision is separate |
| `prepare-srpm.sh` core (RPM Fusion SRPM + kmodtool sign macros, one kernel per rebuild) | reuse, tighten | proven; add retries, take kernels from mirror metadata not the Koji tag, parameterise the Fedora release, fail loudly when the publish probe errors |
| `fetch-kernels.py` | rewrite or drop | the Koji-tag source causes the mirror-lag failures; signed `kernel-devel` from the `updates` repo via `dnf download` is enough |
| `ryoku-nvidia.spec` per-kernel subpackages + `=` userspace pin | reuse the shape | worked; but the main package should not hard-require a kernel that may vanish |
| `Conflicts: kernel-core > X` hold | rewrite | leaks via `kernel-uki-virt`, silent, and turns a stopped pipeline into a frozen machine; needs a fuller hold (all `kernel-uname-r` providers, or a dnf versionlock/exclude managed by the tool) plus a staleness signal |
| `enable-rpmfusion.sh` | reuse, add retry | pinned-key verification is good; one-shot curl is the most common failure |
| Workflow (build / keyless test / COPR publish split) | reuse the structure | the split is what kept the key off COPR and caught bad builds; needs retries and a staleness alert, and an upstream-owned repo/COPR |
| `ryoku-nvidia` host tool `status` / `install` | reuse with changes | GPU gating and akmod-respect worked; queue the key even with Secure Boot off; drop the hard-coded fork COPR |
| `nvidia-fallback.service` preset | reuse | proved itself on the host |
| Doctor reconciler | reuse, extend | add "kernel held because no signed module for N days" and "published package uninstallable" checks |
| ISO provisioner hook | re-test | never exercised on real hardware in the evidence found |

## Open items this research could not settle

- Whether the `kernel-uki-virt` leak produces a bootable entry on a GRUB host
  (`/boot/loader/entries` needs root).
- Whether upstream accepts RPM Fusion as a dependency and a Ryoku-held module key
  (FP-001's open questions).
- What happens to machines already enrolled with the fork key if upstream mints
  its own.
