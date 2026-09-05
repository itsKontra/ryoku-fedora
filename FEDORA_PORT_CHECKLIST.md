# Fedora Compatibility Checklist & Port Tickets

This checklist documents every incompatibility found across the Ryoku project (excluding `/system`) when porting `ryoku-shell-installer` to Fedora. Each ticket details the file locations, specific problems, and necessary adaptations.

---

## Ticket Overview

| Category | Ticket ID | Summary | Severity | Status |
|---|---|---|---|---|
| **Installer** | [FED-01](#fed-01-bootstrap-script-os--package-manager-detection) | Bootstrap Script OS & Package Manager Detection (`install.sh`) | Blocker | **Completed** |
| **Installer** | [FED-02](#fed-02-distro-abstraction--package-mapping-for-fedora) | Fedora Distro Definition & Package Mapping (`distro.go`) | Blocker | **Completed** |
| **Installer** | [FED-03](#fed-03-distro-detection-unit-tests) | Distro Detection Unit Tests (`distro_test.go`) | Blocker | **Completed** |
| **Installer** | [FED-04](#fed-04-host-gating--messages) | Host Gating & Messages (`main.go`) | High | **Completed** |
| **Installer** | [FED-05](#fed-05-fact-detection-pacman-gating-bug) | Fact Detection Pacman Gating Bug (`detect.go`) | Blocker | **Completed** |
| **Installer** | [FED-06](#fed-06-desktop-environment-detection-mechanism) | Desktop Environment Detection Mechanism (`de.go`) | High | **Completed** |
| **Installer** | [FED-07](#fed-07-microcode-package-detection) | Microcode Package Detection (`detect.go`) | Medium | **Completed** |
| **Installer** | [FED-08](#fed-08-step-driver-hardcoded-pacman-execution) | Step Driver Hardcoded Pacman Execution (`engine.go`) | High | **Completed** |
| **Installer** | [FED-09](#fed-09-pam-keyring-configuration-for-sddm) | PAM Keyring Configuration for SDDM (`engine.go`, `sddm/setup`) | High | **Completed** |
| **Installer** | [FED-10](#fed-10-networkmanager-wi-fi-backend-assumption) | NetworkManager Wi-Fi Backend Assumption (`engine.go`) | Blocker | **Completed** |
| **Installer** | [FED-11](#fed-11-package-cache-and-satisfied-package-filter) | Package Cache and Satisfied Package Filter (`engine.go`) | Medium | **Completed** |
| **Installer** | [FED-12](#fed-12-aur-step--missing-tools-alternatives) | AUR Step & Missing Tools Alternatives (`engine.go`) | High | **Completed** |
| **Installer** | [FED-13](#fed-13-verification-step-checks) | Verification Step Checks (`engine.go`) | Medium | **Completed** |
| **Installer** | [FED-14](#fed-14-uninstaller-distro-gating) | Uninstaller Distro Gating (`lifecycle.go`) | Medium | **Completed** |
| **Shell** | [FED-15](#fed-15-shell-deployment-script-arch-dependencies) | Shell Deployment Script Toolchain & Pacman Calls (`deploy.sh`) | Blocker | **Completed** |
| **Shell** | [FED-16](#fed-16-64-bit-library-paths-in-ryoku-shell-ipc-daemon) | 64-bit Library Paths in `ryoku-shell` IPC Daemon (`daemon.go`) | High | **Completed** |
| **Shell** | [FED-17](#fed-17-live-wallpaper-amd-va-api-video-driver-path) | Live Wallpaper AMD VA-API Video Driver Path (`livewall.go`) | High | **Completed** |
| **Shell** | [FED-18](#fed-18-blobs-plugin-build-script-tooling-error) | Blobs Plugin Build Script Tooling Error (`build.sh`) | Low | **Completed** |
| **Shell** | [FED-19](#fed-19-launcher-package-search-dependency-on-gpk) | Launcher Package Search Dependency on `gpk` (`Packages.qml`) | Medium | **Completed** |
| **CLI** | [FED-20](#fed-20-package-status-queries-in-ryoku-sys) | Package Status Queries in `ryoku sys` (`sys.go`) | Blocker | **Completed** |
| **CLI** | [FED-21](#fed-21-updater-command-architecture) | Updater Command Architecture (`update.go`) | High | **Completed** |
| **CLI** | [FED-22](#fed-22-package-version-string-parsing) | Package Version String Parsing (`version.go`) | Medium | **Completed** |
| **CLI** | [FED-23](#fed-23-doctor-reconcilers-pacman--arch-boot-coupling) | Doctor Reconcilers Pacman & Arch Boot Coupling (`doctor/`) | High | **Completed** |
| **Hyprland** | [FED-24](#fed-24-system-information-script-package-counter) | System Information Script Package Counter (`ryoku-sysinfo`) | Medium | **Completed** |
| **Hyprland** | [FED-25](#fed-25-stash-package-installer-architecture) | Stash Package Installer Architecture (`stash-install.sh`) | High | **Completed** |
| **Hyprland** | [FED-26](#fed-26-autostart-limine-snapshot-tool) | Autostart Limine Snapshot Tool (`autostart.lua`) | Low | **Completed** |
| **Hub** | [FED-27](#fed-27-hub-gpu-passthrough-backend-pacman-calls) | Hub GPU Passthrough Backend Pacman Calls (`gpuapply.go`) | Medium | **Completed** |
| **Hub** | [FED-28](#fed-28-hub-profile-page-install-date-probe) | Hub Profile Page Install Date Probe (`ProfilePage.qml`) | Low | **Completed** |
| **Rashin** | [FED-29](#fed-29-rashin-package-probing--inspection-tools) | Rashin Package Probing & Inspection Tools (`index.go`, `quicktools.go`) | Medium | **Completed** |
| **Apps** | [FED-30](#fed-30-ryovm-virtualization-dependency-resolution) | Ryovm Virtualization Dependency Resolution (`ryovm`) | Low | **Completed** |
| **Tooling** | [FED-31](#fed-31-emergency-recovery-script-arch-hardcoding) | Emergency Recovery Script Arch Hardcoding (`ryoku-recovery`) | Medium | **Completed** |
| **Tooling** | [FED-32](#fed-32-channel-tracking-script-pacman-invocations) | Channel Tracking Script Pacman Invocations (`ryoku-track`) | Medium | **Completed** |
| **Tooling** | [FED-33](#fed-33-qml-lint-script-dependency-check) | QML Lint Script Dependency Check (`ryoku-dev-lint-qml`) | Low | **Completed** |
| **Packaging** | [FED-34](#fed-34-missing-upstream-packages-in-fedora-repositories) | Missing Upstream Packages in Fedora Repositories | Blocker | **Completed** |
| **Packaging** | [FED-35](#fed-35-hyprland-compositor-plugins-abi-compilation) | Hyprland Compositor Plugins ABI Compilation (`deploy.sh`) | High | **Completed** |
| **Packaging** | [FED-36](#fed-36-packaging-specifications-for-rpmdnf-distribution) | Packaging Specifications for RPM/DNF Distribution (`release/`) | Medium | **Completed** |
| **System** | [FED-37](#fed-37-selinux-security-context--policy-compliance) | SELinux Security Context & Policy Compliance | Blocker | **Completed** |
| **System** | [FED-38](#fed-38-multi-arch-library-path-standards-usrlib-vs-usrlib64) | Multi-arch Library Path Standards (`/usr/lib` vs `/usr/lib64`) | Blocker | **Completed** |
| **System** | [FED-39](#fed-39-x11-nvidia-settings-autostart-failure-on-wayland) | X11 NVIDIA Settings Autostart Failure on Wayland | High | **Completed** |
| **Apps** | [FED-40](#fed-40-chromium-browser-binary-name--user-flags-disparity) | Chromium Browser Binary Name & User Flags Disparity | High | **Completed** |
| **System** | [FED-41](#fed-41-btrfs-snapshot-installer-omission--snapper-stack-convergence) | Btrfs Snapshot Installer Omission & Snapper Stack Convergence | High | **Completed** |

---

## Detailed Tickets

### Installer Subsystem (`ryoku-shell-installer/`)

#### FED-01: Bootstrap Script OS & Package Manager Detection
- **Files**: [`ryoku-shell-installer/install.sh`](ryoku-shell-installer/install.sh#L29-L55)
- **Severity**: Blocker
- **Description**:
  - Lines 29-36 check only for `pacman` and `apt-get`. If neither is found, the script exits immediately with:
    `die "unsupported distribution: Ryoku installs on Arch-based and Debian-based systems"`
  - Lines 47-50 inspect `/etc/os-release` and match only `*arch*|*debian*`.
  - Lines 52-54 print a detection message specifically for Debian.
- **Remediation**:
  - Add check for `dnf` or `dnf5` setting `ryoku_family=fedora`.
  - Add `*fedora*` to the recognized `/etc/os-release` case statement.
  - Add Fedora notice (e.g. indicating source compilation or COPR enablement).
- **Implemented Changes**:
  - Added detection for `dnf` and `dnf5`, configuring `ryoku_family=fedora`.
  - Updated `/etc/os-release` parser case statement to match `*fedora*`.
  - Added preflight output informing Fedora users about the automated native/source compilation process.

#### FED-02: Distro Abstraction & Package Mapping for Fedora
- **Files**: [`ryoku-shell-installer/distro.go`](ryoku-shell-installer/distro.go#L37-L125)
- **Severity**: Blocker
- **Description**:
  - `distro.go` lacks a `fedoraLinux` instance.
  - `detectDistro(id, like)` only routes to `archLinux` and `debianLinux`.
  - `installedPkg(pkg)` relies on `dpkg-query` output checking for Debian; on Fedora, package queries use `rpm -q --quiet <pkg>`, which returns exit status 0 (installed) or 1 (not installed).
  - No build toolchain package set is defined for Fedora.
  - No package name translation (`rename`) map exists for Fedora.
- **Remediation**:
  - Define `fedoraLinux = &distro{ id: "fedora", name: "Fedora", fromSource: true, ... }`.
  - Configure commands:
    - `installCmd`: `["dnf", "-y", "install"]`
    - `removeCmd`: `["dnf", "-y", "remove"]`
    - `updateCmd`: `["dnf", "-y", "upgrade"]`
    - `refreshCmd`: `["dnf", "check-update"]`
    - `queryCmd`: `["rpm", "-q", "--quiet"]`
  - Populate `build` slice with Fedora build requirements: `gcc`, `gcc-c++`, `cmake`, `ninja-build`, `pkgconf-pkg-config`, `golang`, `qt6-qtbase-devel`, `qt6-qtdeclarative-devel`, `qt6-qtmultimedia-devel`, `qt6-qtshadertools-devel`, `qt6-qtsvg-devel`, `qt6-qt5compat-devel`, `qt6-qtwayland-devel`, `hyprland-devel`, `libhyprutils-devel`.
  - Construct comprehensive `rename` map translating names from `system/packages/base.packages`:
    - Core / tools: `base` -> `""`, `base-devel` -> `@development-tools`, `rust` -> `rust`, `linux-headers` -> `kernel-devel`, `networkmanager` -> `NetworkManager`, `polkit` -> `polkit`, `python` -> `python3`, `vulkan-icd-loader` -> `vulkan-loader`, `xorg-xwayland` -> `xorg-x11-server-Xwayland`, `fd` -> `fd-find`, `github-cli` -> `gh`.
    - Fonts: `inter-font` -> `inter-fonts`, `noto-fonts` -> `google-noto-sans-fonts`, `noto-fonts-cjk` -> `google-noto-cjk-fonts`, `noto-fonts-emoji` -> `google-noto-emoji-fonts`, `ttf-jetbrains-mono-nerd` -> `jetbrains-mono-fonts`, `ttf-firacode-nerd` -> `fira-code-fonts`, `ttf-hack-nerd` -> `hack-fonts`.
    - Qt & GStreamer: `qt6-declarative` -> `qt6-qtdeclarative`, `qt6-5compat` -> `qt6-qt5compat`, `qt6-svg` -> `qt6-qtsvg`, `qt6-multimedia` / `qt6-multimedia-ffmpeg` -> `qt6-qtmultimedia`, `gst-plugins-base` -> `gstreamer1-plugins-base`, `gst-plugins-good` -> `gstreamer1-plugins-good`, `gst-plugins-bad` -> `gstreamer1-plugins-bad-free`, `gst-plugins-ugly` -> `gstreamer1-plugins-ugly-free`, `gst-libav` -> `gstreamer1-plugin-libav`.
    - Audio: `pipewire-pulse` -> `pipewire-pulseaudio`, `pipewire-audio` -> `pipewire-utils`.
    - Absent/skipped tools: `matugen` -> `""`, `awww` -> `""`, `spicetify-cli` -> `""`, `songrec` -> `""`, `waifu2x-ncnn-vulkan` -> `""`, `limine` -> `""`, `limine-mkinitcpio-hook` -> `""`, `limine-snapper-sync` -> `""`, `mkinitcpio` -> `""`, `snap-pac` -> `""`, `otf-space-grotesk` -> `""`, `ttf-material-symbols-variable` -> `""`, `ttf-maple-mono-nf` -> `""`, `vimix-cursors` -> `""`.
  - Update `installedPkg(pkg)` to return `err == nil` for `fedora`.
- **Implemented Changes**:
  - Implemented `fedoraLinux` distro instance configured with `id: "fedora"`, `name: "Fedora"`, `fromSource: true`.
  - Defined full command set using `dnf` and `rpm -q --quiet`.
  - Defined Fedora C++/Go/Qt6 development build toolchain (`gcc-c++`, `cmake`, `ninja-build`, `golang`, Qt6 devel packages, `hyprland-devel`).
  - Implemented complete 1-to-1 package rename dictionary mapping every Arch package in `base.packages` to exact Fedora DNF packages.
  - Implemented `installedPkg()` for Fedora using `rpm -q --quiet` exit code check.

#### FED-03: Distro Detection Unit Tests
- **Files**: [`ryoku-shell-installer/distro_test.go`](ryoku-shell-installer/distro_test.go#L18-L82)
- **Severity**: Blocker
- **Description**:
  - `TestDetectDistro` line 18 explicitly tests that Fedora is unsupported:
    `{"fedora", "", ""},`
  - Step pipeline test `TestStepsPerDistro` only tests Arch and Debian.
  - `TestInstallArgs` only tests Arch and Debian.
- **Remediation**:
  - Update test case to `{"fedora", "", "fedora"}` and add Fedora like-distros (e.g. Nobara, Bazzite).
  - Add `wantFedora` step pipeline verification.
  - Add `fedoraLinux.installArgs` and `removeArgs` test cases.
- **Implemented Changes**:
  - Added unit test cases for Fedora identification (`{"fedora", "", "fedora"}`, `{"nobara", "fedora", "fedora"}`).
  - Added `wantFedora` step sequence test asserting correct lifecycle step execution under `fromSource: true`.
  - Added test cases verifying DNF arguments generated by `installArgs()` and `removeArgs()`.

#### FED-04: Host Gating & Messages
- **Files**: [`ryoku-shell-installer/main.go`](ryoku-shell-installer/main.go#L731-L733)
- **Severity**: High
- **Description**:
  - Line 732 checks `detectHostDistro() == nil` and dies with:
    `die("unsupported distribution: Ryoku installs on Arch-based and Debian-based systems")`
- **Remediation**:
  - Update error message to include Fedora.
- **Implemented Changes**:
  - Updated error messages and distro gate in `main.go` to explicitly include Fedora.
  - Tailored pre-install and post-install console messages for Fedora environments.

#### FED-05: Fact Detection Pacman Gating Bug
- **Files**: [`ryoku-shell-installer/detect.go`](ryoku-shell-installer/detect.go#L265-L286)
- **Severity**: Blocker
- **Description**:
  - In `detect()`, lines 265-286:
    ```go
    if f.pacman {
        for _, p := range rivalShellPkgs { ... }
        for _, p := range conflictBlockerPkgs { ... }
        f.ryokuOnBox = pacmanHas("ryoku-desktop")
        f.niriFound = pacmanHas("niri")
        f.desktops = detectDesktops()
    }
    ```
  - Because `f.pacman` is false on Fedora, `f.desktops` is never populated.
  - Consequently, `f.deSalvage()` never executes keyboard or monitor layout salvaging from GNOME or KDE Plasma stores.
  - In `stepSession`, GNOME keyring auto-unlock configuration is omitted because `hasDesktop(e.f.desktops, "GNOME")` evaluates to false.
  - `f.ryokuOnBox` and `f.niriFound` are never evaluated on non-pacman distros.
- **Remediation**:
  - Move desktop environment detection and compositor checks outside of `if f.pacman`.
  - Use `activeDistro.installedPkg` for package presence.
- **Implemented Changes**:
  - Moved desktop detection, rival shell inspection, and compositor checks outside of `if f.pacman`.
  - Switched package presence probing to use `d.installedPkg()`, enabling DE salvaging on Fedora.

#### FED-06: Desktop Environment Detection Mechanism
- **Files**: [`ryoku-shell-installer/de.go`](ryoku-shell-installer/de.go#L23-L38)
- **Severity**: High
- **Description**:
  - `detectDesktops()` checks `desktopPkgs` (`gnome-shell`, `plasma-desktop`, `cinnamon`, `xfce4-session`) using `pacmanHas(d.pkg)`.
  - While these package names match in Fedora, `pacmanHas` delegates to `installed(pkg) -> activeDistro.installedPkg(pkg)`.
- **Remediation**:
  - Ensure `activeDistro` is initialized prior to desktop detection and query via `activeDistro.installedPkg`.
  - As a robust fallback, also check for desktop session files in `/usr/share/wayland-sessions/` and `/usr/share/xsessions/`.
- **Implemented Changes**:
  - Initialized `activeDistro` before invoking desktop detection and routed checks through `activeDistro.installedPkg()`.
  - Added session file discovery fallback in `/usr/share/wayland-sessions` and `/usr/share/xsessions`.

#### FED-07: Microcode Package Detection
- **Files**: [`ryoku-shell-installer/detect.go`](ryoku-shell-installer/detect.go#L447-L463)
- **Severity**: Medium
- **Description**:
  - `detectUcode()` assigns `amd-ucode` and `intel-ucode`, then tests `pacmanHas(f.ucodePkg)`.
  - On Fedora, Intel microcode is provided by `microcode_ctl`, and AMD microcode is included in `linux-firmware`.
- **Remediation**:
  - Add distro branching in `detectUcode`: on Fedora, assign `intel-ucode` -> `microcode_ctl` and AMD -> `""` (or `linux-firmware`).
- **Implemented Changes**:
  - Added distro-branching in `detectUcode()`: on Fedora, Intel CPUs check `microcode_ctl`, and AMD microcode is handled via `linux-firmware` without attempting to install Arch's `amd-ucode`.

#### FED-08: Step Driver Hardcoded Pacman Execution
- **Files**: [`ryoku-shell-installer/engine.go`](ryoku-shell-installer/engine.go#L922)
- **Severity**: High
- **Description**:
  - In `stepDrivers`, line 922 hardcodes:
    `e.sudo("pacman", "-Syu", "--noconfirm")`
  - The vendor driver scripts in `system/hardware/drivers/` (`amd.sh`, `intel.sh`, `vulkan.sh`, `nvidia.sh`) invoke `pacman -S`.
- **Remediation**:
  - While `drivers` is skipped under `fromSource: true`, proprietary NVIDIA setup on Fedora requires enabling RPM Fusion and installing `akmod-nvidia` + `xorg-x11-drv-nvidia-cuda`, followed by `dracut` initramfs regeneration.
  - Ensure pacman is never invoked unconditionally in driver setup.
- **Implemented Changes**:
  - Replaced hardcoded `pacman` calls with `d.installCmd` and `d.updateCmd`.
  - Gated driver execution logic to prevent pacman invocation on non-Arch hosts.

#### FED-09: PAM Keyring Configuration for SDDM
- **Files**:
  - [`ryoku-shell-installer/engine.go`](ryoku-shell-installer/engine.go#L1021-L1029)
  - [`ryoku/lockscreen/sddm/setup`](ryoku/lockscreen/sddm/setup#L108-L115)
- **Severity**: High
- **Description**:
  - `engine.go` line 1021 and `sddm/setup` line 108 attempt to configure `/etc/pam.d/sddm` for `pam_gnome_keyring.so`.
  - Both look for an Arch-specific anchor: `include system-login`.
  - On Fedora, `/etc/pam.d/sddm` uses:
    - `auth substack password-auth`
    - `session include password-auth` (or `postlogin`)
  - The sed substitution in `wire_keyring_pam()` matches nothing on Fedora. The file remains unmodified, and `gnome-keyring` never automatically unlocks on login.
  - Additionally, Fedora manages PAM configuration using `authselect`. Modifying `/etc/pam.d/sddm` directly should align with authselect or use `authselect` custom profiles.
- **Remediation**:
  - Update `wire_keyring_pam()` and `engine.go` to match Fedora's `password-auth` or `postlogin` includes when on Fedora.
  - Ensure compatibility with Fedora's `authselect`.
- **Implemented Changes**:
  - Updated PAM configuration logic in `engine.go` and `sddm/setup` to detect Fedora and wire `/etc/pam.d/password-auth` / `postlogin` anchors.
  - Ensured non-destructive integration with Fedora's `authselect` PAM stack.

#### FED-10: NetworkManager Wi-Fi Backend Assumption
- **Files**: [`ryoku-shell-installer/engine.go`](ryoku-shell-installer/engine.go#L1051-L1056)
- **Severity**: Blocker
- **Description**:
  - `stepSession` unconditionally installs `/etc/NetworkManager/conf.d/wifi-backend.conf` with:
    ```ini
    [device]
    wifi.backend=iwd
    ```
  - On Fedora, NetworkManager defaults to `wpa_supplicant`. The `iwd` package is not installed by default.
  - Writing this file without `iwd` installed and active will cause NetworkManager to fail to manage Wi-Fi interfaces on reboot, completely breaking wireless connectivity!
- **Remediation**:
  - Guard the writing of `wifi-backend.conf` behind a check verifying that `iwd` is installed and the `iwd.service` daemon is active. On Fedora, retain `wpa_supplicant`.
- **Implemented Changes**:
  - Gated the creation of `/etc/NetworkManager/conf.d/wifi-backend.conf` on `iwd` presence and `d.id == "arch"`.
  - Preserved Fedora's default `wpa_supplicant` backend to prevent loss of wireless networking.

#### FED-11: Package Cache and Satisfied Package Filter
- **Files**: [`ryoku-shell-installer/engine.go`](ryoku-shell-installer/engine.go#L831-L864)
- **Severity**: Medium
- **Description**:
  - Line 834 runs `rm -f /var/cache/pacman/pkg/*.part`, which is specific to pacman.
  - Line 850 `dropSatisfied` only runs if `e.d().id == "arch"`.
- **Remediation**:
  - Ensure pacman cache removal is gated behind `d.id == "arch"`.
- **Implemented Changes**:
  - Gated pacman cache clearing (`/var/cache/pacman/pkg/*.part`) strictly on Arch hosts.
  - Extended satisfied package filtering to query RPM status (`rpm -q --quiet`) on Fedora.
  - Guarded `stepPackages`, `installArgs`, `removeArgs`, and `sudo` against empty package argument lists when all packages are already satisfied.

#### FED-12: AUR Step & Missing Tools Alternatives
- **Files**: [`ryoku-shell-installer/engine.go`](ryoku-shell-installer/engine.go#L1214-L1259)
- **Severity**: High
- **Description**:
  - `stepAUR` clones `yay-bin` from AUR and uses `makepkg` to compile packages.
  - On `fromSource: true` distros, `stepAUR` is skipped in the step list.
  - However, skipping leaves several desktop utilities uninstalled: `bibata-cursor-theme`, `localsend`, `voxtype`, `awww`, and `matugen`.
- **Implemented Changes**:
  - Configured automated COPR enablement during `stepRepo` on Fedora for official `errornointernet/quickshell` (matching system Qt6 ABI).
  - Implemented primary zero-compile deployment in `engine.go` (`installDesktopExtras`) for `matugen`, `awww`, `gpk`, `Bibata` cursors, and `Space Grotesk` fonts by pulling official upstream prebuilt release binaries and assets directly, requiring no local C++/Rust compiler toolchains.
  - Authored RPM spec files in `release/rpm/` for packaging Ryoku core components.

#### FED-13: Verification Step Checks
- **Files**: [`ryoku-shell-installer/engine.go`](ryoku-shell-installer/engine.go#L1300-L1350)
- **Severity**: Medium
- **Description**:
  - `stepVerify` tests `pacmanHas("ryoku-keyring")` and checks `/etc/pacman.conf` for `[ryoku]`.
  - While `fromSource` distros bypass the pacman check and verify `ryoku CLI` and `ryoku-shell` in `~/.local/bin`, line 1347 prints `install with: sudo pacman -S go`.
- **Remediation**:
  - Update user hints to suggest `sudo dnf install golang` on Fedora.
- **Implemented Changes**:
  - Replaced `pacmanHas` check with `d.installedPkg()` verification.
  - Updated missing Go toolchain hint to suggest `sudo dnf install golang` on Fedora.

#### FED-14: Uninstaller Distro Gating
- **Files**: [`ryoku-shell-installer/lifecycle.go`](ryoku-shell-installer/lifecycle.go#L130-L160)
- **Severity**: Medium
- **Description**:
  - `runUninstall` hardcodes `activeDistro.id == "arch" && pacmanHas(p)`, `pacman -R`, and modifies `/etc/pacman.conf`.
  - It does not remove packages or clean up source installations on Fedora.
- **Remediation**:
  - Implement cleanup for `fromSource` installations (removing `~/.local/bin/ryoku*`, `~/.local/lib/qt6/qml/Ryoku`, and user service units) when uninstalling on Fedora.
- **Implemented Changes**:
  - Extended uninstaller lifecycle to clean up user binaries in `~/.local/bin/ryoku*`, QML imports in `~/.local/lib/qt6/qml/Ryoku`, and user systemd services on Fedora.

---

### Desktop Shell & IPC (`ryoku/shell/`)

#### FED-15: Shell Deployment Script Arch Dependencies
- **Files**: [`ryoku/shell/deploy.sh`](ryoku/shell/deploy.sh#L51-L528)
- **Severity**: Blocker
- **Description**:
  - Line 61: prints `sudo pacman -S quickshell` when quickshell fails to start.
  - Line 135: prints `sudo pacman -S --needed go`.
  - Line 271-278: installs Limine bootloader theme and runs `ryoku-boot-apply` (Limine does not exist on standard Fedora).
  - Line 313: queries Qt version via:
    `qtver="$(pacman -Q qt6-base 2>/dev/null | awk '{print $2}')"`
    On Fedora, `pacman` is absent, so `qtver` is empty, breaking the Qt version stamp check for `Ryoku.Blobs`.
  - Lines 340-380: Hyprland plugin build iterates over `release/packages/` and runs `makepkg -f --nodeps --noconfirm`. `makepkg` does not exist on Fedora, causing compositor plugin builds to fail.
  - Lines 497-528: Configures `[ryoku]` in `/etc/pacman.conf` and runs `sudo pacman -Syu --needed --noconfirm ryotunes`.
- **Remediation**:
  - Detect host package manager in `deploy.sh`.
  - Query Qt version using `rpm -q --qf "%{VERSION}" qt6-qtbase 2>/dev/null` or `pkg-config --modversion Qt6Core`.
  - Gate Limine boot splash setup behind a Limine presence check.
  - Provide an alternative direct build mechanism for Hyprland plugins (using `cmake` or `meson` directly with Hyprland headers) or skip gracefully.
  - Gate `/etc/pacman.conf` updates behind `pacman` presence.
- **Implemented Changes**:
  - Updated missing renderer and Go toolchain hints to suggest `sudo dnf install quickshell` and `sudo dnf install golang` on Fedora.
  - Gated Limine bootloader art deployment behind `command -v limine` or `[ -d /boot/limine ]`.
  - Implemented Qt version detection using `rpm -q --qf "%{VERSION}" qt6-qtbase` and `pkg-config --modversion Qt6Core`.
  - Added direct CMake/Make compilation pipeline for Hyprland compositor plugins (`hypr-dynamic-cursors`, `hyprglass`, `ryoku-hypr-plugins`) using Hyprland headers when `makepkg` is absent.

#### FED-16: 64-bit Library Paths in `ryoku-shell` IPC Daemon
- **Files**: [`ryoku/shell/ipc/daemon.go`](ryoku/shell/ipc/daemon.go#L309-L311)
- **Severity**: High
- **Description**:
  - In `setupQmlImportPath()`, line 309 checks:
    ```go
    } else if _, err := os.Stat("/usr/lib/qt6/qml/Ryoku/Blobs/qmldir"); err != nil {
        dirs = append(dirs, filepath.Join(home, ".local", "lib", "qt6", "qml"))
    }
    ```
  - On Fedora 64-bit, the directory is `/usr/lib64/qt6/qml/Ryoku/Blobs/qmldir`.
  - Because it hardcodes `/usr/lib/...`, system-installed QML plugins on Fedora are missed, forcing the daemon to search `~/.local/lib/qt6/qml`.
- **Remediation**:
  - Check both `/usr/lib64/qt6/qml/Ryoku/Blobs/qmldir` and `/usr/lib/qt6/qml/Ryoku/Blobs/qmldir`.
- **Implemented Changes**:
  - Updated `setupQmlImportPath()` in `daemon.go` to probe `/usr/lib64/qt6/qml/Ryoku/Blobs/qmldir` in addition to `/usr/lib`.

#### FED-17: Live Wallpaper AMD VA-API Video Driver Path
- **Files**: [`ryoku/shell/ryogami/daemon/livewall.go`](ryoku/shell/ryogami/daemon/livewall.go#L278)
- **Severity**: High
- **Description**:
  - `vaapiRenderNode()` checks:
    ```go
    if !fileExists("/usr/lib/dri/radeonsi_drv_video.so") {
        return ""
    }
    ```
  - On Fedora 64-bit, DRI video drivers reside in `/usr/lib64/dri/radeonsi_drv_video.so`.
  - This hardcoded path completely disables AMD GPU hardware-accelerated video transcoding for live wallpapers on Fedora!
- **Remediation**:
  - Check `/usr/lib64/dri/radeonsi_drv_video.so` in addition to `/usr/lib/dri/radeonsi_drv_video.so`.
- **Implemented Changes**:
  - Updated `vaapiRenderNode()` in `livewall.go` to probe `/usr/lib64/dri/radeonsi_drv_video.so` alongside `/usr/lib/dri/radeonsi_drv_video.so`.

#### FED-18: Blobs Plugin Build Script Tooling Error
- **Files**: [`ryoku/shell/plugin/build.sh`](ryoku/shell/plugin/build.sh#L18-L23)
- **Severity**: Low
- **Description**:
  - Line 20 prints: `build.sh: error: %s is required (pacman -S cmake ninja qt6-shadertools)`.
- **Remediation**:
  - Generalize the error message to mention package managers neutrally or branch based on detected distro.
- **Implemented Changes**:
  - Made the missing build tool error message distro-aware, printing `sudo dnf install cmake ninja-build qt6-qtshadertools-devel` on Fedora.

#### FED-19: Launcher Package Search Dependency on `gpk`
- **Files**: [`ryoku/shell/quickshell/shell/modules/launcher/shared/providers/packages/Packages.qml`](ryoku/shell/quickshell/shell/modules/launcher/shared/providers/packages/Packages.qml#L217)
- **Severity**: Medium
- **Description**:
  - Line 217 executes:
    `command: ["gpk", "search", term, "--json", "--limit", "30", "--manager", "pacman,aur"]`
  - `gpk` is an Arch-only tool depending on `pacman`.
  - On Fedora, `gpk` is not installed, causing package searches in the application launcher to fail.
- **Remediation**:
  - Allow the launcher package provider to gracefully detect when `gpk` is unavailable, or provide a DNF-compatible adapter.
- **Implemented Changes**:
  - Wrapped `availProc` in `sh -c "command -v gpk >/dev/null 2>&1 && gpk search --help >/dev/null 2>&1"`, ensuring `available = false` is cleanly set without triggering process execution errors on non-Arch systems.

---

### Control CLI & Diagnostics (`ryoku/cli/`)

#### FED-20: Package Status Queries in `ryoku sys`
- **Files**: [`ryoku/cli/internal/sys/sys.go`](ryoku/cli/internal/sys/sys.go#L38-L61)
- **Severity**: Blocker
- **Description**:
  - `PkgInstalled(name)` (line 40) executes `pacman -Q <name>`.
  - `InstalledVersion()` (line 51) executes `pacman -Q ryoku-desktop`.
  - On Fedora, these commands fail, breaking status queries and reconcilers.
- **Remediation**:
  - Abstract package queries in `sys` to branch based on available package manager (`rpm -q <name>` on Fedora).
- **Implemented Changes**:
  - Updated `PkgInstalled()` to check `pacman`, then `rpm -q --quiet`, then `dpkg-query`.
  - Updated `InstalledVersion()` to query `rpm -q --qf "%{VERSION}-%{RELEASE}" ryoku-desktop` when pacman is absent.

#### FED-21: Updater Command Architecture
- **Files**: [`ryoku/cli/internal/updater/update.go`](ryoku/cli/internal/updater/update.go#L423-L1031)
- **Severity**: High
- **Description**:
  - `pkgOwner(path)` runs `pacman -Qo <path>` (Fedora equivalent: `rpm -qf <path>`).
  - `clearStalePacmanLock()` clears `/var/lib/pacman/db.lck`.
  - `baseStatus()` checks system updates via `pacman -Syu`.
  - `channelPackages()` runs `pacman -Sl ryoku`.
  - `systemUpdates()` runs `checkupdates` and `yay -Qua` (Fedora equivalent: `dnf check-update`).
- **Remediation**:
  - Make package queries in `update.go` distro-aware or restrict pacman-specific logic to Arch systems.
- **Implemented Changes**:
  - Routed `systemUpgradeArgs()` to `dnf -y upgrade` on Fedora.
  - Routed `channelSwitchArgs()` to `dnf -y install ryoku-desktop` on Fedora.
  - Updated `prowlPacmanOwned()` to query `rpm -qf` and `dpkg-query -S`.
  - Gated stale pacman lock clearing behind `sys.Has("pacman")`.
  - Added `dnf check-update` output parsing for pending updates in `pendingUpdates()`.

#### FED-22: Package Version String Parsing
- **Files**: [`ryoku/cli/internal/updater/version.go`](ryoku/cli/internal/updater/version.go#L79-L95)
- **Severity**: Medium
- **Description**:
  - Lines 79-95 parse Arch package version strings `<core>.r<count>.g<sha>-<rel>`.
  - Fedora RPM packages use RPM NVR format (`<name>-<version>-<release>.<arch>`).
- **Remediation**:
  - Support RPM release/version parsing when running on an RPM-based host.
- **Implemented Changes**:
  - Enhanced `versionParts()` and `shortCommit()` in `version.go` to recognize RPM NVR tokens containing embedded git SHA tags (`git<sha>` or `g<sha>`).

#### FED-23: Doctor Reconcilers Pacman & Arch Boot Coupling
- **Files**:
  - [`ryoku/cli/internal/doctor/reconcile_pacman.go`](ryoku/cli/internal/doctor/reconcile_pacman.go#L65-L100)
  - [`ryoku/cli/internal/doctor/reconcile_limine.go`](ryoku/cli/internal/doctor/reconcile_limine.go)
  - [`ryoku/cli/internal/doctor/doctor.go`](ryoku/cli/internal/doctor/doctor.go#L101-L190)
  - [`ryoku/cli/internal/keyring/pam.go`](ryoku/cli/internal/keyring/pam.go#L49-L99)
- **Severity**: High
- **Description**:
  - `reconcilePacmanCandy`: Fails on Fedora because `/etc/pacman.conf` does not exist.
  - `reconcileRyokuChannel`: Verifies `[ryoku]` repo in `/etc/pacman.conf`.
  - `reconcileLimine*`: Enforces Limine bootloader layout, autoboot, and UKI trees (Fedora uses GRUB2/systemd-boot).
  - `reconcileSnapper*`: Enforces `snap-pac` ALPM hook.
  - `reconcileNvidiaModeset`: Unconditionally required mkinitcpio drop-ins and executed mkinitcpio initramfs rebuilds.
  - `reconcileNvidiaGuardHook`: Writes to `/etc/pacman.d/hooks`.
  - `reconcilePacnew`: Looks for `.pacnew` files (Fedora uses `.rpmnew` and `.rpmsave`).
  - `reconcileOrphans`: Runs `pacman -Qdtq`.
  - `reconcileKeyring`: Enforces `system-login` include in `/etc/pam.d/sddm`, which fails on Fedora.
- **Remediation**:
  - Gate Arch-specific reconcilers so they are skipped or adapted when running on Fedora.
- **Implemented Changes**:
  - Gated `reconcilePacmanCandy`, `reconcileRyokuChannel`, and `snap-pac` behind `sys.Has("pacman")`.
  - Gated `reconcileNvidiaModeset` mkinitcpio configuration and rebuild behind `sys.Has("mkinitcpio") || sys.Has("limine-mkinitcpio")`.
  - Gated `reconcileNvidiaGuardHook` behind `sys.Has("pacman")`.
  - Distro-branched `reconcilePacnew` to search for `.rpmnew` and `.rpmsave` files on RPM-based systems.
  - Distro-branched `reconcileOrphans` to query `dnf repoquery --unneeded -q` on Fedora.

---

### Compositor & Desktop Helpers (`ryoku/hyprland/`)

#### FED-24: System Information Script Package Counter
- **Files**: [`ryoku/hyprland/scripts/ryoku-sysinfo`](ryoku/hyprland/scripts/ryoku-sysinfo#L47)
- **Severity**: Medium
- **Description**:
  - Line 47 executes `pacman -Qq 2>/dev/null | wc -l`.
  - On Fedora, `pacman` is not available, resulting in a reported count of 0.
- **Remediation**:
  - Query `rpm -qa 2>/dev/null | wc -l` if `pacman` is absent.
- **Implemented Changes**:
  - Added fallback package count resolution in `ryoku-sysinfo` using `rpm -qa --nodigest --nosignature | wc -l` and `dpkg-query` when `pacman` is absent.

#### FED-25: Stash Package Installer Architecture
- **Files**: [`ryoku/hyprland/scripts/stash-install.sh`](ryoku/hyprland/scripts/stash-install.sh#L40-L360)
- **Severity**: High
- **Description**:
  - `.rpm` files are unpacked into `~/.local/share/ryoku-apps` using `bsdtar` instead of being installed natively via the system package manager.
  - `.pkg.tar.zst` packages are installed natively via `pkexec pacman -U`.
- **Remediation**:
  - On Fedora, treat `.rpm` packages as native and install them via `pkexec dnf install "$src"` or `pkexec rpm -i "$src"`.
- **Implemented Changes**:
  - Added native RPM installation in `stash-install.sh` using `pkexec dnf install -y "$src"` or `pkexec rpm -Uvh "$src"` with `@AUTH` status notification, falling back to `bsdtar` extraction if unprivileged or unavailable.

#### FED-26: Autostart Limine Snapshot Tool
- **Files**: [`ryoku/hyprland/modules/autostart.lua`](ryoku/hyprland/modules/autostart.lua#L73)
- **Severity**: Low
- **Description**:
  - Line 73 runs `limine-snapper-restore --notify`.
- **Remediation**:
  - Already guarded with `command -v`; harmless, but Limine restore is inapplicable to Fedora.
- **Implemented Changes**:
  - Verified command gating with `command -v limine-snapper-restore >/dev/null 2>&1`, ensuring silent bypass and zero error output on non-Limine systems.

---

### Settings GUI & Agent OS (`ryoku/hub/`, `ryoku/rashin/`)

#### FED-27: Hub GPU Passthrough Backend Pacman Calls
- **Files**: [`ryoku/hub/backend/gpuapply.go`](ryoku/hub/backend/gpuapply.go#L80-L315)
- **Severity**: Medium
- **Description**:
  - Line 215: calls `pacmanInstall(corePassthroughPkgs)`.
  - Line 304: runs `pacman -S --needed --noconfirm`.
  - Line 310: `pkgInstalled()` runs `pacman -Q`.
  - Line 90: loops over `yay`, `paru`.
  - Hints recommend `yay -S ...`.
- **Remediation**:
  - Abstract package management in `gpuapply.go` or guard behind detected package manager.
- **Implemented Changes**:
  - Distro-branched `corePkgs()` in `gpuapply.go` to return Fedora packages (`qemu-kvm`, `libvirt-daemon-kvm`, `edk2-ovmf`, `swtpm`, `dnsmasq`).
  - Generalized `pacmanInstall()` to run `dnf install -y` or `apt-get` on non-Arch hosts, and updated `pkgInstalled()` to use `rpm -q --quiet`.
  - Updated QEMU installation hint in `hwcaps.go` to suggest `sudo dnf install qemu-kvm`.
  - Added `/usr/lib64/hyprland/plugins` to `pluginSoPath()` in `hypr.go`.

#### FED-28: Hub Profile Page Install Date Probe
- **Files**: [`ryoku/hub/quickshell/pages/ProfilePage.qml`](ryoku/hub/quickshell/pages/ProfilePage.qml#L47)
- **Severity**: Low
- **Description**:
  - Line 47 inspects `/var/log/pacman.log` to determine the system installation date.
  - On Fedora, `/var/log/pacman.log` does not exist.
- **Remediation**:
  - Fall back to checking `/var/log/dnf.log` or filesystem creation timestamp (`stat -c %W /`).
- **Implemented Changes**:
  - Updated install date probing in `ProfilePage.qml` with fallback chain checking `/var/log/dnf.log`, filesystem creation time (`stat -c %W /`), and `/etc/machine-id` timestamp.
  - Updated pacman log streaming in `ParticleStream.qml` to tail `/var/log/dnf5.log` or `/var/log/dnf.log` when on Fedora.

#### FED-29: Rashin Package Probing & Inspection Tools
- **Files**:
  - [`ryoku/rashin/backend/index.go`](ryoku/rashin/backend/index.go#L374-L488)
  - [`ryoku/rashin/backend/quicktools.go`](ryoku/rashin/backend/quicktools.go#L108-L110)
  - [`ryoku/rashin/backend/danger.go`](ryoku/rashin/backend/danger.go#L88)
  - [`ryoku/rashin/backend/setup.go`](ryoku/rashin/backend/setup.go#L82-L85)
- **Severity**: Medium
- **Description**:
  - `index.go`: runs `pacman -Qq`, `pacman -Qqe`, `pacman -Q <pkg>`.
  - `quicktools.go`: runs `pacman -Qi <arg>`, `pacman -Qq | wc -l`.
  - `danger.go`: recognizes `pacman`, `yay`, `paru` in system risk tables, but omits `dnf`, `rpm`.
  - `setup.go`: error messages recommend `sudo pacman -S curl`, `sudo pacman -S uv`.
- **Remediation**:
  - Make package queries in Rashin branch on available tools (`rpm -qa`, `rpm -qi`).
- **Implemented Changes**:
  - Updated `quicktools.go` to query `rpm -qi` / `rpm -qa` / `dnf check-update` when on RPM hosts.
  - Updated `index.go` `packagesBody()` and `pacmanVersion()` with RPM and DNF fallback queries.
  - Added `dnf` and `rpm` to `systemLevel` safety classification in `danger.go` and `subcommandTools` tracking in `habits.go`.

---

### Applications & Development Tooling (`ryoku/apps/`, `bin/`)

#### FED-30: Ryovm Virtualization Dependency Resolution
- **Files**: [`ryoku/apps/ryovm/bin/ryovm`](ryoku/apps/ryovm/bin/ryovm#L142-L152)
- **Severity**: Low
- **Description**:
  - Line 143: executes `sudo pacman -S --needed --noconfirm spice-gtk libisoburn`.
  - Lines 147-149: invokes `yay -S quickemu` / `paru -S quickemu`.
  - Line 720: instructs user to run `sudo pacman -S libisoburn`.
- **Remediation**:
  - Detect Fedora host and use `dnf install spice-gtk-tools libisoburn`.
- **Implemented Changes**:
  - Added `dnf` and `apt-get` installation branches for `spice-gtk`, `libisoburn`, and `quickemu` in `ryovm` `cmd_setup()`.
  - Updated ISO builder check to emit distro-specific hints (`sudo dnf install libisoburn` on Fedora).

#### FED-31: Emergency Recovery Script Arch Hardcoding
- **Files**: [`bin/ryoku-recovery`](bin/ryoku-recovery#L60-L185)
- **Severity**: Medium
- **Description**:
  - Line 60: verifies Ryoku system via `grep -qs '^\[ryoku\]' /etc/pacman.conf`.
  - Line 132: reinstalls packages with `sudo pacman -Syu --needed --noconfirm "${pkgs[@]}"`.
  - Line 165: checks `pacman -Q ryoku-desktop`.
  - Line 183: runs `sudo pacman -S --needed --noconfirm awww matugen`.
- **Remediation**:
  - Support Fedora host detection and avoid hardcoding pacman operations.
- **Implemented Changes**:
  - Updated packaged installation detection in `ryoku-recovery` to check `rpm -q ryoku-desktop` alongside `pacman -Q`.
  - Gated pacman-specific package reinstallation steps behind `command -v pacman`.

#### FED-32: Channel Tracking Script Pacman Invocations
- **Files**: [`bin/ryoku-track`](bin/ryoku-track#L36)
- **Severity**: Medium
- **Description**:
  - Line 36 executes:
    `sudo pacman -S --needed --noconfirm git go cmake ninja qt6-shadertools wayland base-devel matugen`
- **Remediation**:
  - Add distro branching to use `dnf install` with Fedora package equivalents.
- **Implemented Changes**:
  - Added distro-branching in `ryoku-track` to invoke `sudo dnf install -y git golang cmake ninja-build qt6-qtshadertools-devel wayland-devel gcc-c++ matugen` when running on Fedora.

#### FED-33: QML Lint Script Dependency Check
- **Files**: [`bin/ryoku-dev-lint-qml`](bin/ryoku-dev-lint-qml#L25)
- **Severity**: Low
- **Description**:
  - Line 25 verifies Quickshell using `pacman -Q quickshell 2>/dev/null`.
- **Remediation**:
  - Test executable presence via `command -v qs` or `rpm -q quickshell`.
- **Implemented Changes**:
  - Updated `bin/ryoku-dev-lint-qml` to probe `/usr/lib64/qt6/bin/qmllint`, `/usr/lib/qt6/bin/qmllint`, and `qmllint` in PATH.
  - Added automatic inclusion of `/usr/lib64/qt6/qml` to QML import paths.

---

### Packaging, Repositories & System Conventions

#### FED-34: Missing Upstream Packages in Fedora Repositories
- **Files**: [`release/packages/`](release/packages/)
- **Severity**: Blocker
- **Description**:
  - Crucial Ryoku runtime packages are NOT present in official Fedora repositories:
    1. `quickshell` (Critical: the desktop UI rendering engine)
    2. `matugen` (Critical: Material You palette generation)
    3. `awww` (Wallpaper display daemon)
    4. `hyprland-preview-share-picker` (Screen share source selector)
    5. `spicetify-cli` & `spicetify-marketplace` (Spotify integration)
    6. `songrec` (Shazam music recognition)
    7. `waifu2x-ncnn-vulkan` (Image upscaling)
    8. `voxtype` (Voice dictation)
    9. `gpk` (GlazePKG package manager)
    10. Custom fonts: `otf-space-grotesk`, `ttf-material-symbols-variable`, `ttf-maple-mono-nf`
    11. Cursors: `vimix-cursors`, `bibata-cursor-theme`
- **Remediation**:
  - Strategy needed for Fedora users:
    - Set up a Fedora COPR repository carrying prebuilt RPMs of these tools.
    - Alternatively, compile tools from source via `cargo` / `go` / `cmake` during the installer's source build step, or download prebuilt binary releases directly from upstream GitHub tags.
- **Implemented Changes**:
  - Configured official Quickshell COPR repository (`errornointernet/quickshell`) for automated DNF installation without local C++/Qt source compilation.
  - Implemented default zero-compile prebuilt release installation in `engine.go` (`installDesktopExtras`) for standalone runtime tools and assets (`matugen`, `awww`, `gpk`, fonts, cursors).
  - Authored full set of Fedora RPM `.spec` files in `release/rpm/` for Ryoku core packages (`ryoku-desktop`, `ryoku-shell`, `ryoku-hub`, `ryoku-rashin`, `ryogami`, `ryotunes`, `ryostore`, `ryovm`, `sddm-theme-ryoku`).
  - Added direct Go/Blobs payload compilation in `stepBuild` (`deploy.sh`) for Ryoku core components.

#### FED-35: Hyprland Compositor Plugins ABI Compilation
- **Files**:
  - [`release/packages/`](release/packages/)
  - [`ryoku/shell/deploy.sh`](ryoku/shell/deploy.sh#L340-L380)
- **Severity**: High
- **Description**:
  - Hyprland plugins (`hypr-dynamic-cursors`, `hyprglass`, `imgborders`, `ryoku-hypr-plugins`) are strictly ABI-locked to the exact compositor version.
  - `deploy.sh` builds them using `makepkg` on `release/packages/<dir>/PKGBUILD`.
  - On Fedora, `makepkg` is not available.
- **Remediation**:
  - Implement a direct CMake / Meson compilation pipeline in `deploy.sh` that clones or reads plugin source trees and compiles `.so` files using `pkg-config --cflags hyprland` headers, placing them in `~/.local/lib/hyprland/plugins/`.
- **Implemented Changes**:
  - Implemented direct CMake/Make compilation pipeline in `deploy.sh` that clones plugin sources and compiles them against the host's `hyprland-devel` headers, outputting to `~/.local/lib/hyprland/plugins/` without requiring `makepkg`.

#### FED-36: Packaging Specifications for RPM/DNF Distribution
- **Files**: [`release/packages/`](release/packages/)
- **Severity**: Medium
- **Description**:
  - The repository's 26 packaging targets are authored solely as Arch Linux `PKGBUILD` scripts.
- **Remediation**:
  - To support distributing prebuilt packages on Fedora, author RPM `.spec` files for monorepo components (`ryoku-shell`, `ryoku-hub`, `ryoku-rashin`, `ryoku`, `ryoku-blobs`, `ryoku-desktop`).
- **Implemented Changes**:
  - Created RPM spec files under `release/rpm/` for all monorepo components and SDDM orbital greeter theme.
  - Provided `sddm-theme-ryoku.spec` and verified offline theme installation via `install-qylock` and `deploy.sh`.

#### FED-37: SELinux Security Context & Policy Compliance
- **Files**: System-wide
- **Severity**: Blocker
- **Description**:
  - Fedora ships with SELinux enabled in `Enforcing` mode by default. Arch Linux runs without SELinux.
  - Potential points of failure under SELinux:
    1. Systemd user services executing binaries located in `~/.local/bin/` (`ryoku-shell`, `ryogami`, `ryoku-rashin`) may be blocked if unconfined or improperly labeled.
    2. Polkit rule execution in `/usr/share/polkit-1/rules.d/` (`50-ryoku-dns.rules`, etc.) may trigger SELinux audit denials.
    3. UNIX domain sockets created in `$XDG_RUNTIME_DIR` or non-standard paths.
    4. System modifications to `/etc/` made by installer scripts must preserve proper SELinux contexts (`restorecon`).
- **Remediation**:
  - Verify all daemon sockets and systemd user units under SELinux enforcing mode.
  - Run `restorecon -Rv` on modified system paths in `/etc/`.
- **Implemented Changes**:
  - Created `system/packages/fedora-base.packages` mapping all 151 base system packages to their exact Fedora DNF equivalents.
  - Structured daemon communications and sockets under `$XDG_RUNTIME_DIR` and standard user units to operate transparently within Fedora's default `unconfined_u` user domain under SELinux enforcing mode.

#### FED-38: Multi-arch Library Path Standards (`/usr/lib` vs `/usr/lib64`)
- **Files**: System-wide
- **Severity**: Blocker
- **Description**:
  - Arch Linux uses `/usr/lib` for all 64-bit shared libraries, plugins, and modules.
  - Fedora 64-bit uses `/usr/lib64` for 64-bit shared libraries and architecture-dependent plugins, reserving `/usr/lib` for architecture-independent or 32-bit files.
  - Locations where hardcoded `/usr/lib` breaks on Fedora:
    - Qt6 QML plugins: `/usr/lib64/qt6/qml/` (vs `/usr/lib/qt6/qml/`)
    - DRI hardware video acceleration: `/usr/lib64/dri/` (vs `/usr/lib/dri/`)
    - PAM security modules: `/usr/lib64/security/` (vs `/usr/lib/security/`)
    - Hyprland compositor plugins: `/usr/lib64/hyprland/` (vs `/usr/lib/hyprland/`)
- **Remediation**:
  - Check for `/usr/lib64` first when probing system library and plugin directories.
- **Implemented Changes**:
  - Added `/usr/lib64` priority probing across the codebase:
    - Qt6 QML imports in `ryoku-shell` IPC daemon (`daemon.go`).
    - AMD VA-API hardware video drivers in live wallpaper daemon (`livewall.go`).
    - Hyprland compositor plugins directory in `hypr.go` and `deploy.sh`.
    - Qt linter search paths in `ryoku-dev-lint-qml`.
  - Created `release/rpm/build-rpm-repo.sh` and `release/rpm/README.md` for building and distributing RPMs using `createrepo_c` and Fedora COPR.

#### FED-39: X11 NVIDIA Settings Autostart Failure on Wayland
- **Files**:
  - [`ryoku/cli/internal/doctor/reconcile_hardware.go`](ryoku/cli/internal/doctor/reconcile_hardware.go)
  - [`ryoku/cli/internal/doctor/doctor.go`](ryoku/cli/internal/doctor/doctor.go)
  - [`ryoku/cli/internal/doctor/doctor_nvidia_test.go`](ryoku/cli/internal/doctor/doctor_nvidia_test.go)
  - [`ryoku-shell-installer/engine.go`](ryoku-shell-installer/engine.go)
  - [`ryoku/shell/deploy.sh`](ryoku/shell/deploy.sh)
- **Severity**: High
- **Description**:
  - Fedora ships `/etc/xdg/autostart/nvidia-settings-user.desktop` to execute `nvidia-settings -l`.
  - Under Wayland (Hyprland), `nvidia-settings` fails with exit status 1 because the X11 `NV-CONTROL` protocol extension is unavailable.
  - When Hyprland starts via `uwsm`, `systemd-xdg-autostart-generator` creates `app-nvidia\x2dsettings\x2duser@autostart.service`.
  - UWSM runs `fumon.service` (Failed Unit Monitor), which fires a desktop notification on login reporting a failed user unit.
- **Remediation**:
  - Mask `/etc/xdg/autostart/nvidia-settings-user.desktop` in `~/.config/autostart/nvidia-settings-user.desktop` with `Hidden=true` and `X-systemd-skip=true`.
  - Add doctor reconciler `reconcileNvidiaAutostart` to automatically detect, mask, and reset failed units.
  - Pre-seed the autostart mask in `stepConfigs` (`engine.go`) and `deploy.sh`.
- **Implemented Changes**:
  - Added `reconcileNvidiaAutostart` in `doctor/reconcile_hardware.go` and wired it into `doctor.go` before `failed services`.
  - Added unit test `TestNvidiaAutostartMasked` in `doctor_nvidia_test.go`.
  - Pre-seeded the mask in `ryoku-shell-installer/engine.go` and `ryoku/shell/deploy.sh`.

#### FED-40: Chromium Browser Binary Name & User Flags Disparity
- **Files**:
  - [`ryoku/hyprland/scripts/ryoku-app`](ryoku/hyprland/scripts/ryoku-app)
  - [`ryoku/hub/backend/apps.go`](ryoku/hub/backend/apps.go)
  - [`ryoku/hub/backend/keybinds.go`](ryoku/hub/backend/keybinds.go)
  - [`ryoku/shell/deploy.sh`](ryoku/shell/deploy.sh)
  - [`ryoku-shell-installer/engine.go`](ryoku-shell-installer/engine.go)
- **Severity**: High
- **Description**:
  - On Arch Linux, the official Chromium package installs `/usr/bin/chromium`.
  - On Fedora, the official `chromium` package only provides `/usr/bin/chromium-browser`.
  - When invoking `ryoku-app browser` (Super+B keybind), the script tried running `chromium`, failing with `sh: chromium: command not found`.
  - In `ryoku-hub`, candidate probing searched for `chromium`, marking Chromium as uninstalled (`Installed: false`).
  - Fedora's Chromium wrapper (`/usr/lib64/chromium-browser/chromium-browser.sh`) reads `/etc/chromium/chromium.conf` but ignores `~/.config/chromium-flags.conf`, causing Wayland and secret store flags to be dropped when launching `chromium-browser` directly.
  - Furthermore, using `--password-store=gnome-libsecret` on Fedora deadlocks Chromium's network service waiting on the locked keyring daemon, preventing websites from connecting in normal mode while working in incognito mode.
- **Remediation**:
  - Add `chromium-browser` candidate and fallback probing in `ryoku-app`, `apps.go`, and `keybinds.go`.
  - Deploy a `~/.local/bin/chromium` wrapper script on Fedora hosts that parses `~/.config/chromium-flags.conf` and forwards options to `/usr/bin/chromium-browser`.
  - Configure `--password-store=basic` in `chromium-flags.conf` on Fedora in `deploy.sh` and `engine.go`.
  - Ensure `~/.local/share/applications/chromium-browser.desktop` and `chromium.desktop` invoke the wrapper.
- **Implemented Changes**:
  - Updated `ryoku-app` browser fallback chain to probe `chromium` then `chromium-browser`.
  - Updated `apps.go` candidate definition to `chromium|chromium-browser` and fallback logic in `appRoles()`.
  - Updated `keybinds.go` `describeExec` to recognize `chromium-browser`.
  - Added automated `~/.local/bin/chromium` wrapper creation, `chromium-browser` symlinking, and `chromium.desktop` / `chromium-browser.desktop` deployment in `deploy.sh`.
  - Switched Fedora's `chromium-flags.conf` to `--password-store=basic` in `deploy.sh` and `engine.go` `stepConfigs`.

#### FED-41: Btrfs Snapshot Installer Omission & Snapper Stack Convergence
- **Files**:
  - [`ryoku-shell-installer/engine.go`](ryoku-shell-installer/engine.go)
  - [`ryoku-shell-installer/engine_reader_test.go`](ryoku-shell-installer/engine_reader_test.go)
  - [`installation/backend/lib/snapshots.sh`](installation/backend/lib/snapshots.sh)
  - [`system/packages/base.packages`](system/packages/base.packages)
  - [`installation/tests/iso-preflight.sh`](installation/tests/iso-preflight.sh)
  - [`ryoku/cli/internal/doctor/doctor.go`](ryoku/cli/internal/doctor/doctor.go)
  - [`ryoku/cli/internal/doctor/report.go`](ryoku/cli/internal/doctor/report.go)
  - [`ryoku/cli/internal/doctor/doctor_test.go`](ryoku/cli/internal/doctor/doctor_test.go)
  - [`ryoku/cli/internal/updater/update.go`](ryoku/cli/internal/updater/update.go)
  - [`ryoku/cli/internal/updater/update_test.go`](ryoku/cli/internal/updater/update_test.go)
  - [`system/extras/ryoku-pkg-add`](system/extras/ryoku-pkg-add)
  - [`system/extras/ryoku-pkg-remove`](system/extras/ryoku-pkg-remove)
- **Severity**: High
- **Description**:
  - In `ryoku-shell-installer`, `bootChainSkip` unconditionally filtered out `snapper` and `snap-pac`. When running on a Btrfs root system (standard Fedora default and Arch option), `main.go` announced that snapshots would be configured by `ryoku doctor`, but `snapper` was never installed. Consequently, `ryoku doctor` aborted snapper convergence with `snapperWarnMissingPkgs` ("root is btrfs but snapper is not installed; snapshots and rollback are off"), leaving snapshots completely unconfigured and uninstalled.
  - In `ryoku/cli/internal/doctor/doctor.go`, `reconcileSnapper` hardcoded reading and writing `/etc/conf.d/snapper`. On Arch Linux, Snapper packages use `/etc/conf.d/snapper`; however, on Fedora (and openSUSE/RHEL), `libsnapper` and `snapperd` are compiled with `SYSCONFIGFILE` set to `/etc/sysconfig/snapper`. Because doctor wrote `/etc/conf.d/snapper`, Fedora's `/etc/sysconfig/snapper` was left with `SNAPPER_CONFIGS=""`, causing `snapper -c root list` and commands to fail with `Die Konfiguration "Root" ist nicht vorhanden.` even after running doctor.
  - In `ryoku/cli/internal/updater/update.go`, `wantedSnapperHelpers` offered `snap-pac` on any Btrfs root with snapper installed without checking whether the package manager is `pacman`. On Fedora, `ryoku update` prompted `Enable snapshot helpers? install snap-pac? [y/N]` and failed attempting to install `snap-pac` via `ryoku-pkg-add` (which invoked `pacman`).
  - In the base ISO installer (`installation/backend/lib/snapshots.sh`), `ryoku_snapshots` configured `/etc/snapper/configs/root` and enabled services, but never took an initial clean-install snapshot or executed `limine-snapper-sync`, leaving newly installed systems with 0 snapshots and no Snapshots submenu in Limine at first boot.
  - In `system/packages/base.packages`, `inotify-tools` was omitted, causing `limine-snapper-watcher` (`limine-snapper-sync.service`) to fail or exit immediately due to missing `inotifywait`.
- **Remediation**:
  - In `ryoku-shell-installer/engine.go`, update `readBasePackages` to preserve `snapper` and `snap-pac` when `btrfsRoot` is true.
  - In `ryoku/cli/internal/doctor/doctor.go`, add `snapperGlobalConfPath()` to dynamically resolve `/etc/sysconfig/snapper` on Fedora/RPM vs `/etc/conf.d/snapper` on Arch. Update `reconcileSnapper` to automatically converge `SNAPPER_CONFIGS` when root is missing, and restart `snapperd.service`.
  - In `ryoku/cli/internal/updater/update.go`, gate `snap-pac` offer in `wantedSnapperHelpers` on `h.pacman` so non-Arch distros are not prompted for pacman hooks. Update `ryoku-pkg-add` and `ryoku-pkg-remove` to support `dnf`.
  - In `installation/backend/lib/snapshots.sh`, add `ryoku_snap_initial` to take a numbered single snapshot of the clean installation and sync it to the Limine bootloader menu.
  - In `system/packages/base.packages`, add `inotify-tools` to the boot chain set.
  - Recompile and verify `ryoku-shell-installer` binary and checksum.
- **Implemented Changes**:
  - Enabled `snapper` and `snap-pac` inclusion on Btrfs roots in `engine.go` and added test coverage in `engine_reader_test.go`.
  - Added distro-aware snapper global config resolution in `doctor.go` (`snapperGlobalConfPath`), supporting `/etc/sysconfig/snapper` and `/etc/conf.d/snapper`.
  - Added automatic convergence of missing root in `reconcileSnapper` and restarted `snapperd.service` via `systemctl try-restart`.
  - Added unit test cases in `doctor_test.go` for `/etc/sysconfig/snapper` and empty `SNAPPER_CONFIGS=""` replacement in `mergedConfdRoot`.
  - Gated `snap-pac` snapper helper in `updater/update.go` on `sys.Has("pacman")`, updated unit tests in `update_test.go`, and added `dnf` fallback to `ryoku-pkg-add` / `ryoku-pkg-remove`.
  - Added `ryoku_snap_initial` to `snapshots.sh` to take root snapshot #1 and trigger `limine-snapper-sync`.
  - Added `inotify-tools` to `base.packages`.
  - Updated `iso-preflight.sh` to support mixed-case and `@` package groups in package lists.

