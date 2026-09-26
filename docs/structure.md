# Repository structure

Three pillars, one job each. Everything else is documentation or tooling.

- `ryoku/` the desktop that a user runs.
- `system/` the machine the desktop runs on.
- `installation/` how that machine is built.

The golden rule: **every path has one purpose and appears once.** If you need
something that already exists, reference it; do not copy it.

## `ryoku/` the desktop

Deploys into the user's home (`~/.config`, `~/.local/...`) one way. Source of
truth for the live desktop.

- `apps/` one directory per application, holding that app's native config only:
  `kitty/`, `fish/`, `fastfetch/` (plus the `ryoku-fastfetch` launcher), `nvim/`
  (LazyVim), `yazi/`, `starship/`, `nautilus/`, `npm/` (`npmrc`), `pip/`
  (`pip.conf`), `hyprland-preview-share-picker/` (the screen-share source
  chooser xdph launches). `mimeapps.list` sets the default apps and ships to
  `/usr/share/applications/mimeapps.list`, the lowest XDG layer, so a user's own
  `~/.config/mimeapps.list` (what "Set as default" writes) always wins;
  `chromium-flags.conf` pins Chromium's keyring and Wayland backend.
- `hyprland/` the Hyprland config, authored in **Lua**. `hyprland.lua` is the
  entry point and `require`s each module. `keyboard.lua`, `gpu.lua`,
  `monitors.lua` are hardware-managed seeds, and `monitors_user.lua.example` shows
  how to hand-pin a display that autoscale must leave alone. `modules/` is one concern per file
  (`env`, `input`, `displays`, `decoration`, `animations`, `binds`, `ryoshot`,
  `window_rules`, `fullscreen`, `autostart`). `scripts/` holds Hyprland's own
  leaf helpers, the ones bound to the compositor: `ryoku-monitor` (output
  arrangement), `ryoku-workspace`, and the `ryoku-keysounds-import` sample
  importer. The neutral screen and UI helpers the shell drives by bare name (the
  `ryoku-cmd-*` tools, `ryoku-sysinfo`, the recorder chain) moved to the shell
  tree, so every compositor ships them. `plugins/` holds the one compositor
  plugin Ryoku authors, `keysounds` (C++ against the Hyprland plugin API, with
  its `hyprpm.toml` recipe and the sample generator; see
  `docs/hyprland-plugins.md`); it ships as a package, not with the config. The
  rest of the directory deploys to `~/.config/hypr/`.
- `niri/` the niri config, authored in **KDL**. `config.kdl` is the entry point
  and `include`s the rest, in override order: the `keyboard.kdl`, `gpu.kdl`,
  `monitors.kdl` and `monitors_user.kdl` seeds, then the generated
  `settings.kdl` and `rebinds.kdl`, then `user.kdl` as the last word. Every
  seed ships because a missing include is a hard config error in niri, and each
  is comments only: the generated files carry every real setting. There is no
  `modules/` and no `scripts/`, since the neutral settings come from the store
  and the keybind helpers are shell verbs. It deploys to `~/.config/niri/`.
  See `docs/compositors.md`.
- `lockscreen/` `qylock/` (the lock theme and its quickshell lockscreen),
  `install-qylock`, and `sddm/` (the greeter setup).
- `shell/` the desktop shell subsystem: `quickshell/` (the QML UI. Every surface
  runs in-process in a single `qs -c shell` instance under `shell/`, drawn per
  monitor from one scene (`shell.qml`): `modules/` is one directory per surface
  (`bar` the four-edge frame bars with the bounded menu manager, rail status
  popout cards (`bar/popouts/`), the Super+Escape control sidebar and pluggable
  bar styles (`bar/barstyles/`, see `docs/barstyles.md`); then `dock` the app
  dock on the edge opposite the bar (its own surface, shared by every bar style),
  `launcher`, `overview` (Super+Tab), `wallpaper`,
  `visualizer` (a click-through spectrum layer that renders through the shared
  `Ryoku.Ui` spectrum field, keeping only its per-frame band math in its own
  `Motion.qml`), `osd`, `notifications`, `capture`, `confirm`, and `desktop` the
  wallpaper clock and enabled third-party widgets); `services/` holds the shared
  singletons every surface reads, `components/` the shared UI primitives, and
  `utils/` the shared JS. Beside it are `ryoshot`, `welcome` (the first-run
  guided tour), and `plugins` (the third-party shell plugin runtime:
  `discover.sh` merges the catalogue with the user's `plugins.json`, the widget
  host carries desktop placements, and `kit/` is the `Ryoku.PluginKit` QML module
  a plugin imports for the signature look; see `docs/plugins.md`)),
  `plugin/` (`Ryoku.Blobs`, the C++/QML SDF metaball module the frame renders
  with; `build.sh` builds it, and it ships prebuilt), `matugen/` (palette
  templates rendered on every wallpaper change), `qt6ct/` (the Qt icon theme, `qt6ct.conf`),
  `scripts/` (the neutral leaf helpers the UI drives by bare name: the
  `ryoku-cmd-*` screen and recorder tools, `ryoku-sysinfo`/`ryoku-profile-stats`,
  `ryoku-app`, `ryostage`, and the stash `.sh` helpers, shipped with the shell so
  they resolve on PATH under any compositor),
  `systemd/` (the user session target), `ipc/` (`ryoku-shell`, the Go shell
  daemon that supervises the Quickshell components, owns wallpaper/clipboard/
  lock and the GNOME keyring password prompt (it registers as the keyring system
  prompter; see `ipc/prompter.go` and `ipc/secretexchange.go`), and serves the
  control socket). `deploy.sh` and `dev-*.sh` are the live
  dev-loop tools.
- `ui/` `Ryoku.Ui`, the shared QML module the shell, the Hub and the apps all
  import for one look: the design tokens (`Singletons/Tokens.qml`), the shared
  primitives, and the wallpaper palette. It also hosts the desktop spectrum
  renderer, so the wallpaper and the Hub preview draw one geometry:
  `SpectrumField.qml` with the analytic `shaders/spectrum.frag(.qsb)` pass draws
  every look, `Singletons/VizStyles.qml` is the single catalogue of the eleven
  looks, and `lib/spectrum.js` (with `spectrum.test.mjs` beside it) is the pure
  band math, `lib/place.js` (with `place.test.mjs`) the placement math for a box that turns
  and leans.
  Installs to `/usr/lib/qt6/qml/Ryoku/Ui`.
- `i18n/` the translation catalog and the three runtimes that read it. English
  source strings are the keys, so a developer only ever writes English and a
  missing translation shows English rather than nothing: `langs.json` is the one
  language table (35 languages, add one here and nowhere else),
  `catalog/<code>.json` the generated strings, `catalog/overrides/<code>.json`
  the human fixes the generator may never overwrite, `i18n.go`/`langs.go` the Go
  runtime the CLI and both installers link (module `ryoku-i18n`), and `tools/`
  the extractor/translator (`sync.py`, shipped as `/usr/bin/ryoku-i18n`), the
  QML AST wrapper, and `merge3.py`, the key-by-key catalog merge the upstream
  sync uses. Installs to `/usr/share/ryoku/i18n`, which the QML singleton
  (`ui/Singletons/I18n.qml`) and the Go runtime both read. See `docs/i18n.md`.
- `cli/` the user-facing control CLI, one Go program (`ryoku`): `update`,
  `rollback`, `snapshots`, `status`, `materialize` (lay the base configs into
  `~/.config`), and `reload`. It orchestrates dnf, flatpak, and snapper; it does
  not reimplement them. `main.go` is a thin dispatcher over the concerns under
  `internal/`: `updater` (update, status, rollback, channel, run-state,
  materialize, version), `doctor` (the convergent reconcilers, report, and
  `--explain`), and `sys` (the shared exec/package/path/terminal primitives,
  defined once). Per-command reference, user- vs developer-facing, in
  `docs/cli.md`.
- `hub/` Ryoku Settings, the central control-center GUI (`Super + ,`): `backend/`
  (`ryoku-hub`, the Go data plane that reads the keybind legend from the live
  Hyprland config, generates the `settings.lua` overlay (in `user_edits`) from JSON, and
  persists hub state as TOML; it also speaks the OpenRGB SDK, so the Lighting tab
  drives keyboards and mice per device) and `quickshell/` (the native Qt6/QML app,
  a `FloatingWindow` with a grouped nav rail and global fuzzy search, with live
  editors for displays, appearance, device lighting, lockscreen, animations,
  input, keybinds, window and layer
  rules, autostart, environment, the shell, and the desktop widgets). The product is "Ryoku Settings"; the binary and
  config keep the internal `hub` name. Deployed to `~/.config/quickshell/hub`;
  built by the shell's `deploy.sh`.
- `rashin/` Ryoku Rashin, the optional agent OS (off by default): `backend/`
  (`ryoku-rashin`, one Go program that maintains the markdown knowledge vault at
  `~/.local/share/ryoku/rashin/`, serves the embedded dashboard on
  `127.0.0.1:3600`, and bridges the Hermes agent over ACP) with its hand-authored
  web dashboard embedded under `backend/web/` (no build step), and the `rashin`
  terminal command (the same binary under a second name: natural language to a
  ready-to-run command plan on the fish prompt, with a `conf.d/rashin.fish`
  weave). The Hub's `RashinPage.qml` is the control surface (enable, one-click
  Hermes setup, open dashboard); built by the shell's `deploy.sh`. See
  `docs/rashin.md` and `docs/rashin-terminal.md`.
- `assets/` `brand/` the 力 logo and icons, `wallpapers/` the shipped wallpaper
  set (installs to `~/Pictures/Wallpapers`), and `ryodecors/` the decor art the
  `Decor`/`Placard` components render (installs to `~/Pictures/ryodecors`, kept
  current by `ryoku doctor`; bake more with `bin/art/ryodither`).

## `system/` the machine

System-level definition installed into the target.

- `hardware/` hardware policy and helper scripts (shipped to `/usr/bin` by
  `ryoku-desktop`): `gpu/` (`ryoku-gpu`, `ryoku-gpu-detect`, `ryoku-gpu-mux`,
  `ryoku-nvidia` the signed NVIDIA driver installer, udev rule), `display/`
  (`ryoku-monitor`), `audio/` (`ryoku-mic`, the mic-gain normalizer),
  `drivers/` (per-vendor `intel`/`amd`/`vulkan` install scripts), `power/`
  (`ryoku-hw-laptop`,
  the shared laptop detector; `ryoku-idle`, the laptop-gated `hypridle` launcher;
  `ryoku-power`, the battery charge ceiling and PCIe link power). What actually
  moves power draw and temperature, measured, is in `docs/power.md`.
- `containers/` the container runtime policy behind the stash Cobalt engine:
  `ryoku-docker` (the one privileged door, shipped to `/usr/bin`) and its polkit
  rule. Its own directory rather than a corner of `hardware/`, because a
  container runtime is not hardware. It provisions `docker.service`, the `docker`
  group, and the single `ryoku-cobalt` container, and exposes no docker
  passthrough on purpose: the polkit grant is passwordless, so every action is a
  fixed argument vector.
- `extras/` the helpers behind the Hub's Extras section, shipped to `/usr/bin` by
  `ryoku-desktop`: `ryostore-install` (installs, removes, and reports the
  optional bundles from the `ryostore` catalogue), the `ryoku-pkg-*` routing
  wrappers (repo, remove, multilib, and the legacy AUR name, which routes to
  dnf on Fedora), and `ryoku-cmd-present`.
- `packages/` the package manifests used to define and audit the desktop and
  hardware package closure.
- `bootmenu/` the GRUB boot menu, shipped by `ryoku-desktop`: the Ryoku theme
  and `ryoku-grub-menu` that installs it, the `42_ryoku_snapshots` grub.d
  snippet, the snapper plugin and units behind `ryoku boot-menu`, and the
  `90ryoku-snapshot` dracut module that looks at or restores a snapshot from
  the initramfs. See "Snapshots in the boot menu" in `docs/updates.md`.
- `recovery/` the console fallback when the desktop cannot start, shipped by
  `ryoku-desktop`: the `sddm.service` drop-in, `ryoku-console-fallback.service`
  (a text login on tty1 with a recovery banner), `ryoku-console-guard.service`
  (`ryoku boot-guard --console`, after the login screen died), and
  `95-ryoku-console.install`, the kernel-install plugin behind the "Ryoku
  console" boot entries.

## `installation/` the build

- `fedora/` the Fedora 44 Anaconda/Kickstart image: ISO composition, offline RPM
  repository construction, target provisioning, and gated first-boot setup.
- `tests/` container and VM verification for the Fedora repository, packages,
  provisioning, first boot, signatures, channels, and ISO.

## The distribution model

- The desktop ships as signed RPMs from the Ryoku COPR. `ryoku-desktop` is the
  umbrella package and the compositor-specific RPM supplies the selected
  provider.
- The installer configures the COPR and installs `ryoku-desktop` plus its
  compositor package; per-user config is then copied into `~/.config` by
  `ryoku materialize`, which clobbers Ryoku-owned files and prunes dropped ones
  but never touches user files.
- It only ever flows **repo to system**. A change starts in the repo, is built
  into a package, and is installed; nothing is harvested back from a live machine.

## Shared, not duplicated

When two subsystems need the same thing, it lives once and both reference it:
`ryoku-hw-laptop` is the single laptop/desktop detector used by both GPU policy
and the idle policy. Reuse the helper; never re-implement its logic.

## `ryoku-shell-installer/` the no-ISO installer

The standalone way in: a curl-able `install.sh` bootstrap plus the
`ryoku-shell-install` Go TUI that converts an existing Fedora machine into a
Ryoku one: config backup with a generated `restore.sh`, rival-shell and
daemon migration, COPR trust, the desktop set, SDDM/qylock wiring,
`ryoku materialize`. After it runs once the machine updates through
`ryoku update` like any other. The binary and its checksum are committed so
raw.githubusercontent.com serves them with no release infrastructure.

## `release/` packaging

- `rpm/` owns Fedora source RPM preparation, clean Mock rebuilds, payload
  staging, COPR publication, repository configuration, and package verification.
  Each component has one spec and, when needed, one staging recipe under
  `payload/`.
  - `rpm/nvidia/` the Secure Boot signed NVIDIA open modules (`ryoku-nvidia`):
    the build that signs RPM Fusion's module source for the newest Fedora
    kernels, the spec that packages them, and the public module-signing
    certificate. Published to its own COPR project by `publish-nvidia-kmod.yml`.

## Tooling

- `bin/` repo tooling: the release helpers (`ryoku-release` tags a release,
  `ryoku-release-notes` writes its notes), the CI/hook checks (`ryoku-dev-scan-slop`,
  `ryoku-dev-audit-shell-binds`), `ryoku-dev-sync-upstream` (starts an upstream
  Ryoku merge, see `MergeToFedora.md`), and `art/` for art authoring (`ryodither` bakes
  an image or gif into a 1-bit bone-on-transparent decor; `tiling-demos`
  generates the Appearance tiling-layout preview loops).
- `tests/` standalone CI check scripts (install chroot-safety, shell tool
  availability).
- `.github/` the workflows and issue/PR templates; `.githooks/` the commit gates.
- `.woke.yml` the inclusive-language config.
