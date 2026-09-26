# Development

The loop, the gates, and how to add things without breaking the rules.

## The loop

Edit the repo, deploy, test on the running system.

- **Shell (QML + daemon):** `ryoku/shell/dev-run.sh` builds `ryoku-shell` and
  runs it from the checkout (`qs -p`, hot-reload). `ryoku/hyprland/dev-binds.sh on` binds the
  shell keys for the session; `dev-stop.sh` stops it. Your own `~/.config` is not
  touched.
- **Configs:** `ryoku deploy` builds the binaries and lays the repo into
  `~/.config` from a checkout (it runs `ryoku/shell/deploy.sh`); on an installed
  system `ryoku materialize` copies the base config in. Never edit `~/.config`
  and copy back.
- **Shaders:** a `.frag` needs `qsb --qt6 -o <name>.frag.qsb <name>.frag` beside
  it, and the compiled `.qsb` is committed. Quickshell reads a shader file once
  per process, so a rebuilt `.qsb` needs the surface's process restarted; a QML
  hot-reload keeps serving the old one and the change looks like it did nothing.

## Verify before committing

- Lua: `luac -p <file>` parses every changed Lua file.
- Shell scripts: `bash -n <file>`; the pre-commit hook also checks staged scripts.
- Fedora installer: run the focused scripts in `installation/tests/`. Repository,
  signature, first-boot, provisioning, RPM, and ISO checks each have their own
  script; `installation/fedora/README.md` lists the required VM evidence.
- QML: `qmllint` when available.
- Test behavior, not just that it parses. Exercise the actual change on the
  running system.

## Adding things

- **A package:** the right set in `system/packages/` (`base` for everyone,
  `dev` for toolchains, `hardware` per profile, `aur` for the AUR). Prefer the
  official repos over the AUR when both have it.
- **A keybind:** `ryoku/hyprland/modules/binds.lua`.
- **A Hyprland concern:** a new module under `ryoku/hyprland/modules/` plus one
  `require` in `hyprland.lua`. Do not grow an unrelated module.
- **A shell surface:** a new component under `ryoku/shell/quickshell/`, with any
  state wired through `ryoku-shell` (`ryoku/shell/ipc/`).
- **A QML plugin (C++):** build it against the installed Qt and rebuild it on
  every Qt update. Qt's private API carries no cross-version promise, so a stale
  module (or a stale `quickshell` from the AUR) fails to load and takes the whole
  surface down to a black screen. `deploy.sh` stamps `Ryoku.Blobs` with the Qt it
  built against and rebuilds when that changes; `ryoku doctor` reports a renderer
  or module that cannot load.
- **A system helper:** a `ryoku-<thing>` script under `system/hardware/.../`,
  shipped to `/usr/bin` by the `ryoku-desktop` package (its PKGBUILD installs
  every `system/hardware/*/ryoku-*`), and invoked by name from Lua autostart or
  a keybind.

## How a change reaches users

Where a change lives decides whether, and how, it reaches an installed machine.

- **Desktop config and binaries (`ryoku/`)** reach users through `ryoku update`:
  config is re-laid by `ryoku materialize` (override-safe), binaries come from the
  signed COPR repository, published on every push to `main` (`docs/updates.md`,
  "Publishing: the COPR channel").
- **Push, or work on a branch that is not the channel.** `ryoku update` on a
  checkout reconciles the branch it is ON onto `origin/<channel>`: a clean
  fast-forward when it can, and a `git reset --hard` when the branch has diverged
  (`ryoku/cli/internal/updater/channel.go`, `syncChannel`). The channel branch is
  meant to mirror upstream, so **commits sitting unpushed on `unstable-dev` are
  dropped by the next `ryoku update`** (they survive only in the reflog). Push
  them, or keep them on any other branch. Local edits to tracked files are never
  touched: the deploy runs on what is checked out.
- **The installer (`installation/`)** runs once from the ISO. Fixes here reach
  only new installs from a new ISO, never an existing machine.
- **RPM dependency additions** belong in the matching spec under `release/rpm/`
  and must resolve through Fedora or one of `release/rpm/dependency-coprs`.
- **Stateful drift** the declarative layers cannot express (disk layout,
  subvolumes, swap) is healed by an idempotent `ryoku doctor` reconciler that runs
  inside `ryoku update`.

There is no ordered migration ledger. Config is reconciled declaratively by
`materialize`; stateful drift is reconciled by `ryoku doctor`. Reach for a
reconciler only when a fix must change an existing machine's structure and neither
a package nor `materialize` can do it.

### Adding a doctor reconciler

A reconciler is one entry in `reconcilers()` in `ryoku/cli/doctor.go`. It must be
idempotent: report `ok` when the machine already matches the desired state,
otherwise converge (or, under `--check`, report what it would do). It runs on
every `ryoku update`, so keep the check cheap and the fix safe to repeat; auto-fix
only the exact known-safe case and warn on anything unexpected. Retire it once
every supported install has run it, so the set stays small instead of piling up.

## Binaries and package managers

- The desktop ships as signed RPMs built from `release/rpm/`. The Fedora ISO
  installs the same package set from its offline repository, so installed
  targets have no build-toolchain assumption.
- User-level package managers install without root, into `~/.local/bin` (`npm`,
  `pip --user`, `go install`, `cargo install`, `pipx`, `mise`). Do not
  reintroduce root-global installs or assume `sudo`.

## Commit gates

Every commit passes the hooks in `.githooks/`; never use `--no-verify`.

- `commit-msg`: subject is `[area] scope: summary` with area in
  `global | installation | system | ryoku | docs | test | tooling | release`
  (shell uses `[global]`). No em-dash, no authorship/attribution trailer.
- `pre-commit`: no em-dash in text files, valid bash syntax on staged scripts,
  no filler comment lines.
- `pre-push`: shellcheck when installed.

One logical change per commit. Give anything a user would notice a `Note:`
trailer (the release notes are built from them, see `CONTRIBUTING.md`), and keep
the change documented where future readers will look.

## Research

When something is unfamiliar, look it up against primary sources (the Arch Wiki,
the Hyprland wiki, Quickshell and Qt docs, each tool's own docs), cross-check
anything load-bearing, and confirm the result on the running system. Match
existing patterns in the repo over introducing a new one.
