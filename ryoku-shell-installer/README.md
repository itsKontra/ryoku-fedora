# ryoku-shell

Install the Ryoku desktop on an existing machine, without the ISO.

Mutable Fedora 44 x86_64 supports `--install-mode=packages` through the signed
RPM channel. Set the required `RYOKU_RPM_BASE_URL`, `RYOKU_COPR_FINGERPRINT`,
and `RYOKU_RPM_SIGNING_KEY` deployment values first; see
[Fedora delivery](../release/rpm/README.md). No public URL is assumed.
Default `--install-mode=auto` selects packages on Fedora unless a local payload
or custom repository/ref was selected. Use `--install-mode=source` to keep compiling and
deploying the checkout. Packaged installs use DNF for desktop binaries and
extras, and preserve locally edited source artifacts by refusing an unsafe
conversion. An interrupted install resumes only with the same mode/provider.

A source install builds the desktop from the cloned payload with
`ryoku/shell/deploy.sh`. Its dnf dependencies are read from the payload, never
kept here: the `Requires:` of `ryoku-desktop`, the chosen
`ryoku-desktop-<compositor>` and every first-party spec they pull in
(`release/rpm/`), plus the desktop sections of
`installation/fedora/packages.list`, so a source box matches a packaged or ISO
one. `ryoku recovery` installs the same list (`--dependency-list`). Other
distributions are refused.

```bash
curl -fsSL https://raw.githubusercontent.com/itsKontra/ryoku-fedora/main/ryoku-shell-installer/install.sh | bash
```

Headless / unattended:

```bash
curl -fsSL .../install.sh | bash -s -- --yes
```

Preview without changing anything:

```bash
curl -fsSL .../install.sh | bash -s -- --dry-run
```

Remove the Ryoku desktop again:

```bash
ryoku-shell-install --uninstall        # or: ... | bash -s -- --uninstall
```

## What it does

`install.sh` is a dumb bootstrap: it verifies the machine is a supported
x86_64 Fedora (dnf), downloads the prebuilt `ryoku-shell-install`
binary (checksummed) from this directory, and hands it the real terminal.
Everything else is the binary, a bubbletea TUI sharing the ISO installer's
visual language:

1. **Scan** the machine: distro, GPU, Secure Boot state, display manager,
   network stack, installed desktops (GNOME/KDE/Cinnamon/Xfce), rival
   quickshell shells (Noctalia, DankMaterialShell, Caelestia, iNiR), known
   Hyprland rices (ML4W, HyDE, JaKooLit, end-4, Caelestia), conflicting user
   daemons (dunst/mako/waybar/swww/…), a plain Hyprland, niri or sway setup
   to migrate from, keyboard layout, btrfs, and an interrupted previous run to resume.
2. **Plan** review with per-item toggles (NVIDIA drivers, SDDM switch,
   greeter theme, NetworkManager switch, rival-shell removal, monitor-layout
   carry-over, developer toolchain, fish shell); sections group the list when it gets
   long.
3. **Install**, streamed step by step:
   `dnf upgrade` → tools → sparse payload clone → config backup (with a
   generated `restore.sh`) → signed Ryoku repository → conflict removal
   → desktop packages (or the source build's dependencies) → GPU drivers →
   source build or retirement of a previous source deployment →
   SDDM/qylock/network wiring → `ryoku materialize` + seeds (wallpapers,
   brand, keyboard layout salvaged from the old setup) → `ryoku doctor` →
   verify.

Afterwards the machine is a normal Ryoku box: `ryoku update` updates it
forever, `ryoku doctor` heals it, and the signed Ryoku RPM repository
delivers everything. Nothing here ever needs re-running.

Migration policy: rival shells are uninstalled (toggle), conflicting daemons
are disabled but never uninstalled, the old display manager is disabled (not
removed), desktop environments are never uninstalled and stay selectable at
the login screen, niri and sway stay installed as fallback sessions, and
every config the install touches is saved to
`~/.local/state/ryoku/shell-install/backup-<ts>/` first, `restore.sh`
included. Monitor layout and keyboard intent are salvaged with compositor
configs first: Hyprland (`monitor=`/`monitorv2`/`input`, includes and `$vars`
followed) beats niri beats sway, then KDE (`kxkbrc`,
`kwinoutputconfig.json`), then GNOME (gsettings input-sources,
`monitors.xml`), then `localectl`.

Safety gates: non-systemd systems (Artix and friends) are refused outright.
With Secure Boot enforcing, the NVIDIA toggle is forced off and locked,
because Arch kernels reject unsigned DKMS modules and the driver script
denylists nouveau; sign with sbctl or disable Secure Boot, then re-run.
Manjaro requires a typed acknowledgement in the TUI and is refused under
`--yes` unless `RYOKU_ALLOW_MANJARO=1` is set.

Lifecycle: an interrupted run records its completed steps in
`~/.local/state/ryoku/shell-install-state.json`; the next run offers a
resume toggle (automatic with `--yes`) that skips finished steps and
continues the same backup. `--uninstall` removes the ryoku packages, disables
the Ryoku RPM repository, and walks the backup chain newest to oldest,
running each `restore.sh` with confirmation; those scripts also re-enable
the services and display manager their run disabled. Session packages
(sddm, pipewire, NetworkManager, …) are left installed.

## Development

```bash
go build -trimpath -o ryoku-shell-install .   # rebuild
sha256sum ryoku-shell-install > ryoku-shell-install.sha256
go test ./...
```

The binary and its checksum are committed so `install.sh` can fetch them from
raw.githubusercontent.com with no release infrastructure. Test a branch with:

```bash
curl -fsSL https://raw.githubusercontent.com/itsKontra/ryoku-fedora/<branch>/ryoku-shell-installer/install.sh \
  | RYOKU_SHELL_REF=<branch> bash
```

`--payload /path/to/checkout` (or `RYOKU_SHELL_PAYLOAD`) skips the payload
clone and uses a local repo, for iterating without pushing.
