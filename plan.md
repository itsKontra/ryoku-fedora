# Fedora 44 Ryoku ISO Implementation Plan

Tracking plan and milestones for [issue #10](https://github.com/itsKontra/ryoku-fedora/issues/10): building an installation-first Fedora 44 UEFI ISO with Ryoku and niri preinstalled.

---

## Current Status

- [x] **Milestone 0: Console First-Boot Setup & Architecture Baseline** (Completed in [PR #11](https://github.com/itsKontra/ryoku-fedora/pull/11))
  - [x] Resumable interactive console setup (`installation/fedora/firstboot.py`) on tty1 for locale, keymap, timezone, hostname, and root password via `systemd-firstboot`.
  - [x] Separate interactive prompt for pre-created `ryoku` account password via `passwd`.
  - [x] SDDM start gated by `installation/fedora/firstboot-gate.conf` until `/var/lib/ryoku-firstboot/complete` is written.
  - [x] Offline sysroot preparation helper (`installation/fedora/prepare-firstboot.py`) to arm firstboot, clean seeded host settings, and reset machine identity.
  - [x] 10 unit tests in `installation/fedora/tests/test_firstboot.py`.
  - [x] Offline Fedora 44 container prompt/interruption test in `installation/fedora/tests/firstboot-fedora.py`.
  - [x] GitHub Actions workflow `.github/workflows/fedora-firstboot.yml`.
  - [x] Compose path documented and provisioner audit matrix established in `installation/fedora/README.md`.

- [x] **Milestone 1: Offline Desktop Provisioner**
  - [x] Offline desktop provisioner (`installation/fedora/provision-target.py`) for Anaconda `%post` sysroot execution.
  - [x] Pre-creates locked `ryoku` account (home `/home/ryoku`, shell `/usr/bin/fish`, `wheel` membership) and ensures locked `root`.
  - [x] Enforces password-authenticated `sudo` via `/etc/sudoers.d/10-ryoku-wheel` (mode 0440) and strips any `NOPASSWD` directives.
  - [x] Selects Wayland `niri.desktop` session and `Current=ryoku` theme in SDDM drop-ins (`10-ryoku-wayland.conf`, `99-ryoku.conf`), sets cursor fallback, and wires PAM keyring unlock.
  - [x] Enables `sddm.service` and defaults to `graphical.target`.
  - [x] Enables base system services offline: `NetworkManager.service`, `firewalld.service`, `bluetooth.service`.
  - [x] Seeds qylock in-session lockscreen and `clockwork/orbital` theme into `/home/ryoku`.
  - [x] Seeds wallpapers, ryodecors, brand assets, and `.npmrc` from `/usr/share/ryoku`, and packages wallpapers and brand in `ryoku-desktop` PKGBUILD.
  - [x] Seeds vendor MIME defaults (`niri-mimeapps.list` and `mimeapps.list`).
  - [x] Materializes user configuration with recursive `ryoku:ryoku` ownership across `/home/ryoku`.
  - [x] Invokes `prepare-firstboot.py` on target sysroot to arm console setup.
  - [x] Executes offline SELinux relabeling via `setfiles` across `/etc`, `/var`, and `/home/ryoku`.
  - [x] 12 unit tests in `installation/fedora/tests/test_provision.py` (22 fedora tests total).
  - [x] Offline Fedora 44 container provisioning validation in `installation/tests/fedora-provision.sh`.
- [x] **Milestone 2: Offline RPM Package Closure & Repository Setup**
  - [x] Declarative package closure definition (`installation/fedora/packages.list`):
    - Minimal base: `kernel`, `kernel-core`, `kernel-modules`, `systemd`, `systemd-udev`, `systemd-resolved`, `systemd-networkd`, `dracut`, `grub2-efi-x64`, `shim-x64`, `btrfs-progs`, `cryptsetup`, `selinux-policy-targeted`, `setfiles`, `shadow-utils`, `sudo`, `glibc-all-langpacks`, `kbd`, `tzdata`, `dnf5`, `rpm`.
    - Open graphics drivers & firmware: `mesa-dri-drivers`, `mesa-vulkan-drivers`, `vulkan-loader`, `xorg-x11-server-Xwayland`, `mesa-va-drivers`, `mesa-vdpau-drivers`, `linux-firmware`, `amd-ucode-firmware`, `microcode_ctl`.
    - Core utilities: `chromium`, `tmux`, `neovim`, `vim-enhanced`, `bat`, `lua`, `python3`, `alacritty`, `fish`, `fzf`, `less`, `grep`, `ripgrep`, `nano`, `zsh`, `bash`.
    - Base services: `firewalld`, `NetworkManager`, `NetworkManager-wifi`, `bluez`, `pipewire`, `wireplumber`, `sddm`.
    - Desktop packages: `ryoku-desktop`, `ryoku-desktop-niri`, `niri`, `xwayland-satellite`, `quickshell`.
  - [x] RPM Fusion full FFmpeg integration:
    - Pinned Fedora 44 RPM Fusion release packages and signing keys in `installation/fedora/keys/`.
    - Fingerprint verification for Fedora 44 Primary, RPM Fusion Free 2020, RPM Fusion Nonfree 2020, and Ryoku COPR.
    - Explicit transaction solver replacing `ffmpeg-free` with RPM Fusion's full stack (`ffmpeg`, `ffmpeg-libs`, `libavdevice`) during compose without package dropouts.
  - [x] Local repository tooling (`installation/fedora/build-repo.py`):
    - Packaging and assembly tool with `createrepo_c` and SHA256 checksums.
    - Manifest generation (`manifest.json`) recording package NEVRAs, file sizes, and SHA256 hashes.
    - Offline closure verification in isolated empty installroot with `--disablerepo=*` and networking disabled.
  - [x] 16 unit tests in `installation/fedora/tests/test_repo.py` (38 tests in `installation/fedora/tests/` total).
  - [x] Offline container validation script (`installation/tests/fedora-repo.sh`).
  - [x] CI workflow updated in `.github/workflows/fedora-firstboot.yml`.

- [x] **Milestone 3: Anaconda Kickstart & Lorax ISO Builder**
  - [x] Single-source-of-truth Anaconda Kickstart recipe (`installation/fedora/kickstart/ryoku.ks`):
    - UEFI GPT partitioning scheme: 600 MiB FAT32 ESP at `/boot/efi`, 2 GiB ext4 `/boot`, Btrfs with `root` and `home` subvolumes, and Fedora zram swap policy.
    - Strict target safety: explicit target disk selection and user confirmation via `clearpart --none`; unconditional `clearpart --all` strictly forbidden.
    - Interactive LUKS2 disk encryption prompted without embedded secrets.
    - Local media repository configured with `--cost=10`.
    - Package payload matching `packages.list` with `-ffmpeg-free` and related free subpackages excluded to enforce RPM Fusion full stack.
    - `%post --nochroot --erroronfail` section locating and executing `provision-target.py` on `/mnt/sysroot`.
  - [x] Compose pipeline script (`installation/fedora/build-iso.sh`):
    - GPG key verification, repository metadata (`createrepo_c`), and SHA256 manifest generation.
    - Payload staging into `iso_root` (Kickstart, local RPMs, offline desktop provisioner).
    - Hybrid UEFI bootable ISO generation via `xorriso`, `mkksiso`, or `lorax`.
    - Automated SHA256 checksum file and structured provenance record (`provenance.json`).
  - [x] 9 unit tests in `installation/fedora/tests/test_iso.py` (47 tests in `installation/fedora/tests/` total).
  - [x] Container smoke test script (`installation/tests/fedora-iso.sh`) validating Kickstart syntax with `ksvalidator -v F44`, stage-only compose, ISO image creation with `xorriso`, and checksum verification in `--network=none`.
  - [x] CI workflow updated in `.github/workflows/fedora-firstboot.yml`.

- [x] **Milestone 4: Automated UEFI VM Test Harness & Validation**
  - [x] Automated UEFI VM test harness (`installation/tests/fedora-iso-vm.sh`, `installation/tests/fedora-iso-vm.py`):
    - Launches QEMU with OVMF UEFI firmware, 40 GiB virtual disk, and disconnected network (`-nic none`).
    - Drives Anaconda Kickstart installation with test target disk adaptation and captures serial and install logs.
    - Reboots into installed target disk and drives interactive console setup on tty1 (testing prompt order, answers, and interruption handling).
    - Verifies SDDM gate preconditions and unblocking upon setup completion.
    - Verifies graphical session launch into `niri` desktop with Ryoku shell.
    - Verifies SELinux enforcing status and asserts zero denials (`ausearch -m avc`).
    - Supports both unencrypted and LUKS2-encrypted installation paths.
  - [x] 11 unit tests in `installation/fedora/tests/test_vm.py` (58 tests in `installation/fedora/tests/` total).
  - [x] CI workflow for VM testing in `.github/workflows/fedora-iso-vm.yml` and stage-only gate in `.github/workflows/fedora-firstboot.yml`.

---

## Remaining Milestones

### Milestone 1: Offline Desktop Provisioner (Complete)
*Transform an offline, mounted Fedora sysroot into a fully configured Ryoku desktop target (for Anaconda `%post` execution).*

- [x] **Provisioner Script (`installation/fedora/provision-target.py`)**
  - [x] **Accounts & Sudo**: Pre-create `ryoku` account with home `/home/ryoku`, login shell `/usr/bin/fish`, `wheel` group membership, and a locked password; ensure `root` is locked. Configure `sudo` to require password authentication.
  - [x] **Configuration Materialization**: Run `ryoku materialize` as user `ryoku` with explicit `HOME=/home/ryoku` against the target, ensuring all generated and overlay files retain proper `ryoku:ryoku` ownership without needing an active user systemd manager.
  - [x] **Session & Greeter Selection**:
    - Write SDDM configuration drop-in selecting `niri.desktop` as default Wayland session.
    - Configure SDDM greeter theme to `sddm-theme-ryoku` without running live service restarts.
    - Enable `sddm.service` and set `graphical.target` as default.
  - [x] **Lockscreen Setup**: Seed qylock configuration bundle for `ryoku`.
  - [x] **System Services**: Enable base services offline: `NetworkManager.service`, `firewalld.service`, `bluetooth.service`.
  - [x] **Assets & Integration**: Seed desktop entries, MIME defaults, and wallpapers from `/usr/share/ryoku`.
  - [x] **Arm Firstboot**: Invoke `prepare-firstboot.py` on the target sysroot.
  - [x] **SELinux Relabeling**: Run `setfiles` / `restorecon -Rv` using target policy across `/etc`, `/var`, and `/home/ryoku`.
- [x] **Validation**:
  - [x] Container test script (`installation/tests/fedora-provision.sh`) verifying file ownership, permissions, unit links, and firstboot armed state in a clean Fedora 44 container.
  - [x] Unit test suite (`installation/fedora/tests/test_provision.py`).

---

### Milestone 2: Offline RPM Package Closure & Repository Setup (Complete)
*Establish the complete hermetic package payload and resolve all dependencies for disconnected installation.*

- [x] **Package Closure Definition**:
  - [x] Base package set: Fedora 44 Minimal environment, standard Fedora kernel, and open graphics drivers.
  - [x] Core utilities and tools: `chromium`, `tmux`, `neovim`, `vim-enhanced`, `bat`, `lua`, `python3`, `alacritty`, `fish`, `fzf`, `less`, `grep`, `ripgrep`, `nano`, `zsh`, `bash`.
  - [x] Base services: `firewalld`, `NetworkManager`, `bluez`.
  - [x] Ryoku desktop packages: `ryoku-desktop`, `ryoku-desktop-niri`, and runtime closure.
- [x] **RPM Fusion Full FFmpeg Integration**:
  - [x] Pin Fedora 44 RPM Fusion release packages and signing keys.
  - [x] Explicitly resolve transaction replacing Fedora `ffmpeg-free` with full RPM Fusion `ffmpeg` during compose without unexpected package removals.
- [x] **Local Repository Packaging**:
  - [x] Assemble all binary RPMs into a local repository directory using `createrepo_c`.
  - [x] Resolve and verify the complete dependency closure against an empty installroot with networking disabled.

---

### Milestone 3: Anaconda Kickstart & Lorax ISO Builder (Complete)
*Define the Kickstart installer recipe and automate the ISO compose pipeline.*

- [x] **Kickstart Specification (`installation/fedora/kickstart/ryoku.ks`)**:
  - [x] Partitioning scheme:
    - 600 MiB FAT32 ESP at `/boot/efi`.
    - 2 GiB ext4 `/boot`.
    - Remaining disk as Btrfs with `root` and `home` subvolumes mounted at `/` and `/home`.
    - Fedora zram swap policy.
  - [x] Target safety: Require explicit target disk selection and interactive erasure confirmation; forbid unattended `clearpart --all`.
  - [x] Optional LUKS2 disk encryption prompted interactively during installation.
  - [x] `%packages` payload referencing only local media repository (`--cost=10`).
  - [x] `%post --nochroot --erroronfail` section executing the offline provisioner from Milestone 1.
- [x] **Compose Pipeline (`installation/fedora/build-iso.sh`)**:
  - [x] Build Anaconda installer tree and `.treeinfo` using `lorax`.
  - [x] Inject Kickstart and local RPM repository payload.
  - [x] Produce hybrid UEFI bootable ISO using `xorriso` / `mkksiso`.
  - [x] Generate SHA256 checksums, package manifests, and provenance records.

---

### Milestone 4: Automated UEFI VM Test Harness & Validation (Complete)
*Prove end-to-end correctness in automated virtual machines with disconnected network.*

- [x] **VM Test Automation Harness (`installation/tests/fedora-iso-vm.sh`, `installation/tests/fedora-iso-vm.py`)**:
  - [x] Launch QEMU with OVMF UEFI firmware, virtual drive, and disconnected NIC (`-nic none`).
  - [x] Drive Anaconda Kickstart installation to completion.
  - [x] Capture serial console logs and Anaconda install logs.
- [x] **First-Boot & Desktop Verification**:
  - [x] Reboot into installed virtual disk.
  - [x] Drive interactive console setup on tty1 (verify prompt order, answers, and interruption handling).
  - [x] Verify SDDM unblocks only after completion file exists.
  - [x] Verify graphical login into `niri` desktop with Ryoku shell running.
  - [x] Verify SELinux enforcing status and assert zero denials (`ausearch -m avc`).
  - [x] Test both unencrypted and LUKS2-encrypted installation paths.
- [x] **CI Integration**:
  - [x] Add GitHub Actions workflow for scheduled/manual ISO build and VM smoke tests (`.github/workflows/fedora-iso-vm.yml`).
  - [x] Integrate stage-only VM harness validation in `.github/workflows/fedora-firstboot.yml`.
