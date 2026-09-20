# Fedora installation ISO

Implementation of [issue #10](https://github.com/itsKontra/ryoku-fedora/issues/10)
starts here. The console first-boot component and offline desktop provisioner
are implemented and tested against Fedora 44. **There is no Fedora ISO builder
yet.** The existing `installation/iso/` builds Arch media.

## Offline desktop provisioner

`provision-target.py TARGET_ROOT` prepares an offline, mounted Fedora 44 target
sysroot (intended for execution from Anaconda's `%post --nochroot --erroronfail`
section) into a complete Ryoku desktop installation before first boot:

1. **Accounts and Sudo**: Pre-creates the `ryoku` account with home `/home/ryoku`,
   login shell `/usr/bin/fish`, `wheel` group membership, and a locked password;
   ensures `root` is locked. Configures `sudo` with `/etc/sudoers.d/10-ryoku-wheel`
   (mode `0440`) requiring password authentication, and removes any `NOPASSWD` drop-ins.
2. **Session and Greeter Selection**:
   - Writes SDDM Wayland drop-in `/etc/sddm.conf.d/10-ryoku-wayland.conf`.
   - Writes SDDM theme and session drop-in `/etc/sddm.conf.d/99-ryoku.conf`,
     selecting `Current=ryoku` and `Session=niri.desktop`.
   - Sets the default cursor fallback to `Bibata-Modern-Ice` in `/usr/share/icons/default/index.theme`.
   - Wires `pam_gnome_keyring.so` into `/etc/pam.d/sddm` for unlock-on-login.
   - Enables `sddm.service` and sets `graphical.target` as default.
3. **Base System Services**: Enables `NetworkManager.service`, `firewalld.service`,
   and `bluetooth.service` offline.
4. **Lockscreen**: Seeds the qylock in-session lockscreen bundle and `clockwork/orbital`
   theme into `/home/ryoku`, wiring the themes link and setting theme preference.
5. **Assets and Integration**: Seeds desktop entries, vendor MIME defaults
   (`niri-mimeapps.list` and `mimeapps.list`), wallpapers into `~/Pictures/Wallpapers`,
   decor art into `~/Pictures/ryodecors`, brand assets into `~/.local/share/ryoku/assets/brand`,
   and `.npmrc` from `/usr/share/ryoku`.
6. **Configuration Materialization**: Runs `ryoku materialize` as user `ryoku`
   with explicit `HOME=/home/ryoku` against the target, and ensures recursive `ryoku:ryoku`
   ownership across `/home/ryoku`.
7. **First-Boot Setup**: Invokes `prepare-firstboot.py` on the target sysroot.
8. **SELinux Relabeling**: Runs `setfiles` using the target's policy across `/etc`,
   `/var`, and `/home/ryoku`.

From the future Anaconda `%post --nochroot --erroronfail` section:

```sh
python3 /path/on/media/installation/fedora/provision-target.py /mnt/sysroot
```

## First-boot component

`prepare-firstboot.py TARGET_ROOT` arms setup in a fresh, offline, mounted
Fedora 44 target after packages and accounts have been installed. It refuses
the running root, existing passwords and passwordless accounts. The installer
must pre-create `ryoku` with home `/home/ryoku`, shell `/usr/bin/fish`, wheel
membership and a locked password; root must also be locked. It requires the
target's `python3`, `systemd-firstboot`, `passwd` and fish executables. Include
`kbd`, `kbd-misc`, `glibc-all-langpacks` and `tzdata` for the offline choices.

From the future Anaconda `%post --nochroot --erroronfail` integration, after
the target has been populated:

```sh
python3 /path/on/media/installation/fedora/prepare-firstboot.py /mnt/sysroot
```

The helper installs the console program under `/usr/libexec`, a systemd unit,
and an SDDM dependency/guard. These are installer-seeded files, not an edit to
the desktop RPM payload. The installer must apply the target's SELinux file
contexts after seeding, enable SDDM, and select `graphical.target`. This helper
does not create users, enable sudo, select a session or provision the desktop.
It must run only after those other install steps succeed. No existing desktop
installation or RPM upgrade invokes it.

Arming removes the target's seeded locale, keymap, timezone and hostname once,
resets machine identity to `uninitialized`, and links D-Bus identity to
`/etc/machine-id`. It masks Fedora's stock first-boot unit. The custom service
does not depend on `ConditionFirstBoot`, so an interrupted first boot resumes
on the next boot even though systemd already generated a machine ID. Repeating
the preparation command on an armed target preserves its state.

On tty1, the program explicitly prompts through `systemd-firstboot` for locale,
keymap, timezone, **hostname**, and root password. It invokes `passwd` separately
for `ryoku`. Missing answers are required, including passwords; a successful
command that skipped an answer cannot mark setup complete. Imported systemd
credentials cannot supply invisible answers. The running system manager reloads
the locale and the console service applies the keymap before password entry.

Completed choices are detected from the configuration and shadow database. A
reboot between saving a password and recording completion does not reset that
password. The program syncs changes and atomically records
`/var/lib/ryoku-firstboot/complete` only after all six choices exist. SDDM both
requires successful setup and checks the completion file immediately before
starting. There is no automatic login or shipped password. On failure the login
screen stays blocked; reboot to resume. On later boots the completed service
returns without prompting.

This relies on the installer supplying a fresh target and preventing other
first-boot tools from reseeding answers. It is not a general account-recovery
tool. The booted VM gate still needs to verify actual service ordering, console
layout changes, and SELinux policy. An encrypted installation must also test
the initramfs unlock layout when setup selects a different console keymap.

## Tests

Root-free state-machine, target-preparation, and offline provisioning tests:

```sh
python3 -m unittest discover -s installation/fedora/tests -v
```

The [first-boot workflow](../../.github/workflows/fedora-firstboot.yml) prepares
a disposable Fedora 44 container, then runs
`installation/tests/fedora-firstboot.sh` with `--network=none` and
`RYOKU_TEST_DISPOSABLE=1`. Use Podman's `--no-hostname`: systemd atomically
replaces `/etc/hostname`, which cannot work on a container-generated bind mount.
The test is destructive to the container's accounts and settings; never run it
on a workstation or a target intended for installation.

The test drives the actual tools through a pseudo-terminal, kills setup after
the root password is saved, resumes through the user-password prompt, verifies
the saved settings and credentials, and reruns with stdin closed. Passwords are
random per run and terminal transcripts are never printed. Static unit
verification uses a dummy display-manager service; it is not a boot test.

`installation/tests/fedora-provision.sh` runs inside the same disposable container
against an isolated target sysroot to prove offline user creation, sudo policy,
SDDM configuration drop-ins, service enablement, lockscreen and wallpaper assets,
materialization ownership (`ryoku:ryoku`), first-boot arming, and systemd unit verification.

Local evidence, 2026-09-20: Fedora container
`f0ab7f9811e9`, `systemd-259.9-1.fc44.x86_64`,
`shadow-utils-4.19.0-7.fc44.x86_64`, `glibc-2.43-8.fc44.x86_64`,
`kbd-2.9.0-4.fc44.x86_64`. Real prompt/resume and offline provisioner tests passed offline.
Local Podman needed `--security-opt label=disable` to read the checkout mount;
these results provide no evidence of SELinux behavior on an installed machine.

## Selected compose direction

Use Fedora 44's **Lorax Anaconda installation tree**, extended into an
installation DVD with a complete local RPM repository and Kickstart. Lorax
builds the Anaconda `boot.iso` and `.treeinfo`; its output alone is not a complete
offline package source. Preserve that tree, its installer runtime, and Fedora's
signed shim/GRUB/kernel components when adding the package payload and rebuilding
the ISO. Do not use `livemedia-creator --make-iso` as a substitute: that mode
produces a live image. See the upstream [Lorax guide](https://weldr.io/lorax/lorax.html)
and [livemedia-creator guide](https://weldr.io/lorax/livemedia-creator.html).

This path still needs a prototype with Fedora 44's actual Anaconda and Lorax.
No material ISO blocker has been established, and no disk-image fallback has
been selected.

The planned installation contract is:

- x86_64 UEFI, Fedora Minimal package environment, standard Fedora kernel and
  open graphics drivers; no GNOME/KDE desktop environment.
- Anaconda target selection and explicit whole-disk erasure confirmation. Do
  not embed an unattended first-disk choice or unconditional `clearpart --all`
  into the public media. Optional encryption prompts for its passphrase in the
  installer, never in build inputs, logs or the published Kickstart.
- GPT: 600 MiB FAT32 ESP at `/boot/efi`, 2 GiB ext4 `/boot`, remaining space as
  Btrfs with `root` and `home` subvolumes mounted at `/` and `/home`. Optional
  LUKS2 encloses the Btrfs filesystem; the ESP and `/boot` remain unencrypted.
  Use Fedora zram rather than a disk swap partition. This is the proposed
  layout, not yet a tested partition recipe.
- All packages come from media during installation. Resolve the complete
  dependency closure in an empty Fedora 44 installroot during compose, never
  against packages already installed on the build host. Preserve RPM ownership,
  signatures, modes, dependencies and target SELinux labels.
- Include the requested applications (`chromium`, `tmux`, `neovim`, `vim`, `bat`,
  `lua`, `python3`, `alacritty`, `ffmpeg`, `fish`, `fzf`, `less`, `grep`, `ripgrep`,
  `nano`, `zsh`, and bash), base services (`firewalld`, `NetworkManager`, `bluez`),
  signed `ryoku-desktop` and `ryoku-desktop-niri`, and their runtime closure.
  Enable the firewall, NetworkManager and Bluetooth services in the target.
- Full FFmpeg comes from RPM Fusion. Pin the Fedora 44 RPM Fusion release
  packages and verified signing fingerprints. Explicitly solve the replacement
  of Fedora's `ffmpeg-free` libraries with RPM Fusion's full stack during
  compose; reject a transaction that removes required desktop packages. Never
  rely on an unexplained `--allowerasing` at install or first boot.
- Preserve the existing `[ryoku]` release/channel repository contract for
  subsequent updates. The local media repository is an installation source,
  not the installed machine's permanent update URL.

Record the full source commit, Fedora base/container digest, installer-tool
NEVRAs, frozen repository metadata checksums, signing fingerprints, every RPM's
NEVRA/source repository/SHA256, composed tree checksums and final ISO checksum.
Pin or archive these inputs before claiming a repeatable compose. A mutable
Fedora mirror or COPR's latest directory is not a reproducibility record.

## RPM prerequisite check

Read-only inspection on 2026-09-20 found that both the GitHub repository
variables and the `fedora-publish` environment expose
`RYOKU_COPR_FINGERPRINT=7575D19C9D8ECDC212702114ED1C392039749395`.
Neither exposed `RYOKU_RPM_BASE_URL` nor `RYOKU_RPM_SIGNING_KEY`. Issue #4 is
closed, but its closure does not identify a downloadable, signed release tree.
No signed release/channel was verified in this implementation pass.

Before composing, obtain the actual deployment values, verify the release and
repository metadata with the pinned metadata key, verify the exact desktop
RPMs with the COPR key, and prove a fresh dependency transaction. Reuse
`release/rpm/configure-repo.py`, `verify-rpms.py` and `dependency-coprs`; do not
invent another list of Ryoku dependency repositories or point clients directly
at COPR. See [RPM delivery](../../release/rpm/README.md).

## Provisioning audit and next work

The existing shell installer is a running-user installer. Its root refusal,
sudo authentication and user-systemd calls make it unsuitable as a `%post`
provisioner. Materialization covers configuration, not all of its other work.

| Responsibility | Existing implementation | Offline integration still needed |
|---|---|---|
| Packaged configuration | `ryoku materialize`, Fedora RPM container test | Run as `ryoku` with explicit HOME after RPM installation; retain overlay ownership. Its best-effort WirePlumber restart must not require a running user manager. |
| Session and initial provider | Provider RPM session entries, shell installer `stepSession` | Select the provider's packaged session for SDDM; verify generated provider config before enabling graphical login. |
| Greeter | `ryoku/lockscreen/sddm/setup`, `sddm-theme-ryoku` RPM | Extract/reuse offline-safe selection and Wayland greeter configuration, avoiding running-system service commands and copies over RPM-owned theme files. |
| Lockscreen | `ryoku/lockscreen/install-qylock`, packaged bundle | Seed user lock configuration from the installed bundle; verify PAM and lock/unlock under enforcing SELinux. |
| Network | `stepSession` | Enable NetworkManager with Fedora's normal Wi-Fi backend; avoid importing installer Wi-Fi credentials. |
| Wallpapers and brand assets | `stepConfigs`, `/usr/share/ryoku` payload | Seed user data from the installed package, with no repository checkout or download. |
| App integration | `stepConfigs`, provider MIME defaults | Seed npm settings and app entries from authoritative payloads; preserve the vendor MIME layer and audit editor plugins for first-launch downloads. |
| User units | RPM `%post`, `ryoku-bootstrap.service`, session hooks | Verify offline enablement and first-login activation; do not run `systemctl --user` in `%post`. |
| Account and sudo policy | Anaconda account provisioning | Pre-create locked `ryoku`, wheel/fish, password-required sudo; verify effective sudo policy and absence of autologin. |
| Console setup and identity | Files in this directory | Wire preparation after provisioning and before final SELinux labeling; add booted VM tests. |

Next milestones are the signed offline dependency closure, the shared offline
desktop provisioner, and the installation DVD prototype. Then add automated
OVMF VMs with disconnected NICs for both plain and encrypted whole-disk installs.
Exercise erase confirmation, interruption/reboot at every setup step, distinct
machine IDs, Btrfs mounts and encryption unlock, real SDDM ordering, niri/Ryoku,
settings, terminal, lock, audio, portals, firewall and update/overlay preservation.
Retain journals, Anaconda logs, RPM/configuration manifests and SELinux AVCs.

Publish only after those gates pass, with checksums, provenance and installation
instructions. UEFI boot, Secure Boot, GPUs and physical Bluetooth currently have
**no ISO test coverage**. Secure Boot and proprietary NVIDIA support are not
claimed. Keep hardware-only checks separate from VM evidence.
