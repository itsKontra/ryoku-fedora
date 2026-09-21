# Fedora installation ISO

Implementation of [issue #10](https://github.com/itsKontra/ryoku-fedora/issues/10)
starts here. The console first-boot component, offline desktop provisioner,
offline RPM package closure, Anaconda Kickstart installer recipe, and ISO compose
pipeline are implemented and tested against Fedora 44.

## Offline desktop provisioner

`provision-target.py TARGET_ROOT --anaconda` prepares the mounted Fedora 44
sysroot from Anaconda's `%post --nochroot --erroronfail`:

1. **Accounts and Sudo**: Uses the login accounts, home directories, passwords,
   shells and group memberships created in Anaconda. Choose “Make this user
   administrator” in User Creation for password-authenticated wheel sudo.
   No fixed `ryoku` account or password is injected.
2. **Session and Greeter Selection**:
   - Writes SDDM Wayland drop-in `/etc/sddm.conf.d/10-ryoku-wayland.conf`.
   - Writes SDDM theme and session drop-in `/etc/sddm.conf.d/99-ryoku.conf`,
     selecting `Current=ryoku` and `Session=hyprland.desktop`.
   - Sets the default cursor fallback to `Bibata-Modern-Ice` in `/usr/share/icons/default/index.theme`.
   - Wires `pam_gnome_keyring.so` into `/etc/pam.d/sddm` for unlock-on-login.
   - Enables `sddm.service` and sets `graphical.target` as default.
3. **Base System Services**: Enables `NetworkManager.service`, `firewalld.service`,
   and `bluetooth.service` offline.
4. **Lockscreen**: Seeds the qylock in-session lockscreen bundle and `clockwork/orbital`
   theme into each login account’s home, wiring the themes link and setting theme preference.
5. **Assets and Integration**: Seeds desktop entries, vendor MIME defaults
   (`ryoku-mimeapps.list` and `mimeapps.list`), wallpapers into `~/Pictures/Wallpapers`,
   decor art into `~/Pictures/ryodecors`, brand assets into `~/.local/share/ryoku/assets/brand`,
   and `.npmrc` from `/usr/share/ryoku`.
6. **Configuration Materialization**: Runs `ryoku materialize` for each login
   account with its own `HOME`, and assigns the generated files to that account.
7. **Installer Settings**: Preserves Anaconda's keyboard, locale and timezone;
   does not arm console setup or clear settings on the installed target.
8. **SELinux Relabeling**: Relabels `/etc`, `/var` and the login home directories.

```sh
python3 /path/on/media/installation/fedora/provision-target.py /mnt/sysroot --anaconda
```

Without `--anaconda`, the provisioner retains the separate console-firstboot
workflow: it creates a locked `ryoku` account and arms the setup described below.

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
`installation/tests/fedora-firstboot.sh` with `--network=none`,
`--cap-add=SYS_ADMIN` and `RYOKU_TEST_DISPOSABLE=1`. If `/etc/hostname` is a
container bind mount, the test unmounts it so systemd can atomically replace it.
The test is destructive to the container's accounts and settings; never run it
on a workstation or a target intended for installation.

The test drives the actual tools through a pseudo-terminal, kills setup after
the root password is saved, resumes through the user-password prompt, verifies
the saved settings and credentials, and reruns with stdin closed. Passwords are
random per run and terminal transcripts are never printed. Static unit
verification uses a placeholder display-manager service; it is not a boot test.

`installation/tests/fedora-provision.sh` runs inside the same disposable container
against an isolated target sysroot to prove offline user creation, sudo policy,
SDDM configuration drop-ins, service enablement, lockscreen and wallpaper assets,
materialization ownership (`ryoku:ryoku`), first-boot arming, and systemd unit verification.

`installation/tests/fedora-repo.sh` runs inside the disposable container to prove
pinned key verification, repository creation with `createrepo_c`, SHA256 manifest
generation, and dependency closure resolution in an empty installroot with `--network=none`.

`installation/tests/fedora-iso.sh` runs inside the disposable container to prove
Kickstart recipe validation with `ksvalidator -v F44`, stage-only compose tree staging,
hybrid UEFI ISO composition with `xorriso`, and SHA256 checksum/provenance verification
with `--network=none`.

`installation/tests/fedora-iso-vm.sh` drives end-to-end testing in QEMU with OVMF
UEFI firmware and disconnected network (`-nic none`):
1. **Kickstart Installation**: Boots the composed ISO against a 40 GiB virtual disk,
   capturing serial console and Anaconda install logs.
2. **First-Boot Console Setup**: Drives interactive setup on tty1 through
   `systemd-firstboot` and `passwd` (locale, keymap, timezone, hostname, root password,
   and ryoku user password), verifying interruption handling and answer preservation.
3. **SDDM Gating**: Asserts that `sddm.service` remains gated until
   `/var/lib/ryoku-firstboot/complete` is written, and unblocks successfully once complete.
4. **Desktop Launch**: Verifies graphical session launch into Hyprland Wayland desktop
   with Ryoku Quickshell running.
5. **SELinux Policy**: Asserts enforcing status (`getenforce` is `Enforcing`) and
   confirms zero AVC denials via `ausearch -m avc`.
6. **Encryption**: Validates both plain partitioning and LUKS2-encrypted Btrfs paths.

Local evidence, 2026-09-20: Fedora container
`cb17dd9ebff1`, `systemd-259.9-1.fc44.x86_64`, `shadow-utils-4.19.0-7.fc44.x86_64`,
`glibc-2.43-8.fc44.x86_64`, `kbd-2.9.0-4.fc44.x86_64`, `dnf5-5.4.5.0-1.fc44.x86_64`,
`createrepo_c-1.2.1-1.fc44.x86_64`, `xorriso-1.5.8-2.fc44.x86_64`, `pykickstart-3.69-1.fc44.noarch`.
Real prompt/resume, offline provisioning, offline repository, ISO compose, and UEFI VM harness tests passed offline.

## Anaconda Kickstart and ISO compose pipeline

`kickstart/ryoku.ks` specifies the Anaconda installation recipe for Fedora 44:
1. **Target Safety**: Enforces `clearpart --none` and omits hardcoded drive selections. Unattended whole-disk wiping is strictly forbidden.
2. **UEFI GPT Partitioning**:
   - 600 MiB FAT32 ESP at `/boot/efi`
   - 2 GiB ext4 dedicated `/boot`
   - Remaining disk as Btrfs with `root` and `home` subvolumes mounted at `/` and `/home`
   - Fedora zram swap policy (no disk swap partition)
3. **Interactive Encryption**: Supports LUKS2 encryption prompted interactively without embedded secrets.
4. **Offline Package Repository**: Registers the staged `repo/` directory at `/run/install/repo/repo` with `--cost=10`.
5. **Packages Payload**: Selects `@^ryoku-desktop-environment` and uses Fedora `ffmpeg-free`, as required by the published Ryoku RPMs.
6. **Offline Provisioning**: Executes `provision-target.py --anaconda` on `/mnt/sysroot` during `%post --nochroot --erroronfail`.

The generated environment is named **Ryoku Desktop**. Its required groups are
Fedora `core` (provided by the enabled Fedora repository) and `ryoku-desktop`;
every entry in `packages.list` is mandatory in the latter. Opening Software
Selection and accepting Ryoku Desktop therefore keeps the manifest selection.
Selecting another environment deliberately changes the package selection.
Username and password are entered in User Creation; Keyboard remains editable.
No password is shipped in the recipe.

`build-iso.sh` orchestrates the compose pipeline:
1. **Preflight and Verification**: Checks dependencies, verifies pinned GPG keys, and validates the Kickstart recipe.
2. **Repository Staging**: Generates `comps.xml` from `packages.list` and refreshes metadata with `createrepo_c --update --groupfile`, computes `manifest.json`, and verifies the offline dependency closure.
3. **Payload Staging**: Stages Kickstart, local RPMs, offline provisioner, and stamped media metadata into `iso_root`.
4. **Hybrid ISO Composition**: Produces hybrid UEFI bootable media using `mkksiso`, `xorriso`, or `lorax`.
5. **Checksums and Provenance**: Computes the final `.sha256` checksum file and generates structured `provenance.json` recording build environment, tool versions, and git commit details.

## Offline package closure and repository setup

`packages.list` defines the hermetic package payload required for offline media
installation, organized into categories:
- `[base]`: Fedora 44 Minimal environment, systemd, glibc-all-langpacks, shadow-utils, sudo, selinux-policy-targeted, btrfs-progs, cryptsetup, kbd, tzdata, dnf5, rpm.
- `[kernel]`: Standard Fedora kernel and generic dracut initramfs.
- `[bootloader]`: UEFI bootloader components (`grub2-efi-x64`, `shim-x64`, `efibootmgr`).
- `[drivers]`: Open graphics drivers (`mesa-dri-drivers`, `mesa-vulkan-drivers`, `vulkan-loader`, `xorg-x11-server-Xwayland`) and firmware (`linux-firmware`, `amd-ucode-firmware`, `microcode_ctl`).
- `[utilities]`: Core utilities: `chromium`, `tmux`, `neovim`, `vim-enhanced`, `bat`, `lua`, `python3`, `alacritty`, `fish`, `fzf`, `less`, `grep`, `ripgrep`, `nano`, `zsh`, `bash`.
- `[services]`: Base system services: `firewalld`, `NetworkManager`, `bluez`, `pipewire`, `wireplumber`, `sddm`.
- `[multimedia]`: Fedora FFmpeg (`ffmpeg-free`) and RPM Fusion release packages.
- `[desktop]`: Ryoku desktop components (`ryoku-desktop`, `ryoku-desktop-hyprland`) and compositor runtime (`hyprland`, `quickshell`).

`build-repo.py` manages offline repository construction and verification:
1. **Key Verification**: Validates all pinned GPG keys in `keys/` against hardcoded fingerprints:
   - Fedora 44 Primary: `36F612DCF27F7D1A48A835E4DBFCF71C6D9F90A6`
   - RPM Fusion Free 2020: `E9A491A3DE247814E7E067EAE06F8ECDD651FF2E`
   - RPM Fusion Nonfree 2020: `79BDB88F9BBF73910FD4095B6A2AF96194843C65`
   - Ryoku COPR: `7575D19C9D8ECDC212702114ED1C392039749395`
2. **Netinstall Transaction Solving**: `verify-netinstall.py` checks the complete environment against the configured repositories, including the Fedora FFmpeg dependencies of published Ryoku RPMs.
3. **Repository Metadata**: Runs `createrepo_c` with SHA256 checksums to create standard RPM-MD metadata.
4. **Manifest Generation**: Inspects all packages, computing file sizes, SHA256 digests, and NEVRA records into `manifest.json`.
5. **Offline Closure Verification**: Validates the composed repository in an empty, isolated installroot with `--disablerepo=*` and networking disabled.

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
  `lua`, `python3`, `alacritty`, `ffmpeg-free`, `fish`, `fzf`, `less`, `grep`, `ripgrep`,
  `nano`, `zsh`, and bash), base services (`firewalld`, `NetworkManager`, `bluez`),
  signed `ryoku-desktop` and `ryoku-desktop-hyprland`, and their runtime closure.
  Enable the firewall, NetworkManager and Bluetooth services in the target.
- Use Fedora `ffmpeg-free` for compatibility with the published Ryoku RPMs.
  Validate the complete transaction before composing the ISO; do not use
  `--allowerasing` or skip broken dependencies to hide conflicts.
- Preserve the `[RyokuCOPR]` release/channel repository contract for
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
machine IDs, Btrfs mounts and encryption unlock, real SDDM ordering, Hyprland/Ryoku,
settings, terminal, lock, audio, portals, firewall and update/overlay preservation.
Retain journals, Anaconda logs, RPM/configuration manifests and SELinux AVCs.

Publish only after those gates pass, with checksums, provenance and installation
instructions. UEFI boot, Secure Boot, GPUs and physical Bluetooth currently have
**no ISO test coverage**. Secure Boot and proprietary NVIDIA support are not
claimed. Keep hardware-only checks separate from VM evidence.

### Netinstall dependency validation

Before composing network installation media, pass `--verify-netinstall`
(alongside `--skip-closure-verify`, which applies only to offline RPM payloads).
This resolves the selected environment and every manifest package in an empty
installroot using exactly the repositories in Kickstart. It requires DNF5 and
pykickstart, downloads repository metadata, and does not install packages.
Missing packages or unsatisfied dependencies abort the build.

The ISO enables every dependency COPR listed in `release/rpm/dependency-coprs`.
`policycoreutils` supplies `setfiles`; it is not a separate Fedora package.
The removed `mesa-va-drivers` and `mesa-vdpau-drivers` package names are not
available in the configured Fedora 44 repositories. The Mesa DRI/Vulkan stack
remains selected; Fedora’s `mesa-libgallium` supplies the VA-API driver files.
Fedora's FFmpeg packages match the current published Ryoku
RPM requirements; forcing full RPM Fusion FFmpeg conflicts with those RPMs.
