# Updates and delivery

How a change in this repo reaches a running machine, and the contract that keeps
a user's install a mirror of a dev checkout. Read this before adding a config
file, a `shell.json` key, or anything a user must receive.

## Two lanes, and why

Ryoku owns its own layer and nothing under it. Two package lanes, deliberately
separate:

| Lane | Command | What moves |
|---|---|---|
| **Ryoku** | `ryoku update` | the packages the signed `RyokuCOPR` repository serves, the config, the doctor |
| **Your distribution** | `sudo dnf upgrade` | the base system and its kernel, from Fedora |

`ryoku update` upgrades the installed Ryoku packages by name
(`dnf --repo=RyokuCOPR distro-sync <pkg>...`) and never runs a system upgrade.
The reasons are the design:

- **The kernel is not ours to move.** Ryoku runs on Fedora's kernel and
  publishes none. A Ryoku release must not decide when your box changes kernel,
  rebuilds its modules, or rewrites its boot image.
- **A release has to be reversible.** A snapshot can put the Ryoku set back; an
  update that also moved Fedora was never fully reversible.
- **The lanes fail apart.** A box that cannot take a Fedora upgrade today (a
  mirror out of sync, a full boot partition) must still be able to take a Ryoku
  fix, and the reverse.

So a plain `sudo dnf upgrade` is expected, supported, and the only thing that
moves your kernel. Ryoku ships no hook that blocks it. Every `ryoku update`
reports what that lane is holding (`N system package(s) waiting`), `ryoku
status` prints it as `system:`, and the Hub lists it under SYSTEM PACKAGES;
`ryoku update --system` runs both lanes in one command for those who want that.

One opt-in exception: a Fedora box running the Secure Boot signed NVIDIA driver
(`ryoku-nvidia`) holds kernels newer than the newest one it has a signed module
for. The package conflicts with them, so the distribution lane skips that
kernel until the module is published instead of booting it without a GPU
driver. Ryoku still decides nothing about when the kernel moves; it only waits
for the module. See `system/hardware/README.md`.

The desktop package's scriptlets also make a direct `sudo dnf upgrade` safe
while graphical sessions are live. `%pre` takes one durable login1 sleep block
while the old lid, idle and shell owners are intact; `%posttrans` reloads
logind, then each Hyprland or niri user session (never the SDDM greeter or
another desktop) stops the old lid/idle/shell owners and must report its new
sleep guard ready before the block is released. RPM runs the incoming
package's own scriptlets, so the first release that ships the helper adopts
live sessions in the same transaction.

- A **dev box** runs the checkout: `ryoku deploy` builds the binaries and lays
  `ryoku/` into `~/.config`. `ryoku update` on it tracks `origin/main` (the git
  channel) and redeploys.
- A **user box** runs signed packages: `ryoku update` moves the Ryoku set,
  then `ryoku materialize`, then `ryoku doctor`.

They must converge. A change that lands on one but not the other is the bug this
page exists to prevent.

### Ryotunes

On Fedora, Ryotunes is an RPM from its own COPR project (`itskontra/ryotunes`,
enabled as a dependency repository) and moves with `sudo dnf upgrade`. When the desktop expects it and it is missing,
`ryoku doctor` installs it with `dnf install ryotunes` and enables its socket.
The Arch build tracked prebuilt packages from the
[ryoku-dev/ryotunes](https://github.com/ryoku-dev/ryotunes) GitHub releases
(`internal/ryotunesrelease`); that path installs with pacman and does not apply
to Fedora.

## `ryoku update`

Snapper pre-snapshot, then the channel (git fast-forward, or the `RyokuCOPR`
package set), then stage2 through the just-installed binary. The package's
`%posttrans` first adopts every live Ryoku session; when that adoption did not
complete, stage2 invokes the same all-session helper synchronously. For the invoking active session, stage2
holds a durable login1 sleep block, stops the old shell, idle and clamshell
owners, materializes config, binds `ryoku-session.target` to the exact login1
session, and requires the compositor's power bindings to reload. It starts the
new shell, waits for `sleep-ready`, verifies idle and clamshell, and restores a
previously running Ryogami before releasing protection. Any earlier failure
leaves the durable block active until retry or reboot. Longer `ryoku doctor` and
index work follows. A snapper post-snapshot closes the run. Each stage
publishes to `$XDG_RUNTIME_DIR/ryoku-update.json` (the ordered steps, the current
label, a live log tail, and, on failure, the error and the pre-update snapshot),
so the update island and the Hub's Updates page render a determinate run and a
one-click rollback.

The metadata refresh happens before the set is read, so the update only ever
asks for packages the repository serves now; the transaction is restricted to
`--repo=RyokuCOPR`, so dnf takes our build of a name that also exists in the
Fedora repositories, and `distro-sync` moves it down as readily as up.

After the desktop is back, the update refreshes the agent OS when it is present:
`ryoku-rashin index` regenerates the vault and re-indexes the config mirror with
Prowl, then `prowl` is brought current. On a dev box (Prowl on PATH but
not owned by an RPM) it runs `prowl update`; a packaged box already got the new
build from the Ryoku set, so the step just logs that the binary is managed by
the package manager. Both are best effort and never fail an update.

### The boot guard

A packaged update that moves the box to another release arms a boot guard:
stage2 writes `/var/lib/ryoku/update-pending.json` (previous release, new
release, the pre-update snapshot, the boot it ran in). `ryoku-boot-guard.service`
runs `ryoku boot-guard` as root early in every boot, before the display
manager, and only while that marker exists. The shell daemon records a good
boot once the shell has stayed up 45 s (`/var/lib/ryoku/boot/ok-<uid>`, the boot
id); a record from any boot other than the one the update ran in disarms the
guard. Without one, the boot counts: on the second, the guard tracks the
previous release back (`ryoku track <from>`, then an explicit
reinstall of `ryoku-desktop`; the Ryoku set only, the base system untouched),
re-materializes every user's config from it, and leaves
a notice `ryoku doctor` shows once. On a third it points the Limine boot menu
at the pre-update snapshot entry, for the case where the packages were not what
broke.

The revert steps are still the Arch build's: they run pacman and edit
`/boot/limine.conf`. Fedora has one rolling COPR channel with no release tags to
track back to, and boots through GRUB, so on a Fedora box the guard can arm but
cannot revert yet. `sudo ryoku boot-guard --disarm` clears a marker by hand. The `ryoku`
package ships the unit and its tmpfiles entry; the doctor enables the unit and
prepares the record directory on every update, so boxes installed before it get
it on their next update.

### The console fallback

When the desktop cannot start, the box lands on a text login on tty1 with a
banner naming `ryoku doctor` and `ryoku rollback`, instead of a black screen
(`system/recovery/`). Root stays locked and `SYSTEMD_SULOGIN_FORCE` is never
set, so recovery is a wheel user signing in and using `sudo`.

- An `sddm.service` drop-in sets `OnFailure=ryoku-console-fallback.service`.
  systemd also fires OnFailure on each crash sddm restarts from, so the
  fallback's `ExecCondition` only goes on when sddm is really `failed` or
  `inactive`. It then writes `/etc/issue.d/ryoku-console-fallback.issue` and
  starts `getty@tty1`. A tmpfiles `r!` line clears the banner on the next boot.
- A login screen that dies does not fail sddm: sddm stays up and does not
  restart the greeter, leaving a black screen. The same drop-in pulls in
  `ryoku-console-guard.service` before sddm. It runs
  `ryoku boot-guard --console`, which reads the previous boot's sddm journal:
  when the last `sddm-greeter` PAM session closed while sddm was not stopping,
  and neither a new greeter nor a login followed, it writes
  `/run/ryoku/console-boot` (the drop-in's `ConditionPathExists=!` then skips
  sddm for this boot) and starts the fallback. The boot after that tries the
  desktop again.
- `95-ryoku-console.install`, a kernel-install plugin, adds a "Ryoku console"
  BLS twin of every kernel entry with `systemd.unit=multi-user.target`. GRUB
  orders entries by their version with no `~` rule, so the twin's version is
  `0-ryoku-console-<kver>`: the console entries list below every kernel and
  above rescue. `%post` twins the kernels already installed; `%preun` removes
  the twins.
- The shell daemon runs `grub2-set-bootflag boot_success` with its good-boot
  record, and `ryoku-desktop` turns off Fedora's `grub-boot-success.timer`
  (which sets the flag two minutes into any login), so a boot whose desktop
  never came up shows the GRUB menu, with the console entries, next time.

All of it ships in `ryoku-desktop` and needs no unit enabled, so existing boxes
get it on their next `ryoku update`.

## materialize: the config a user receives

`ryoku materialize` lays the package's base config (`/usr/share/ryoku/config`,
mirrored by `ryoku/shell/deploy.sh` on a dev box) into `~/.config`:

- Every shipped file is copied over on every update (the previous Ryoku copy is
  clobbered) and files dropped from a release are pruned; `~/.config/quickshell`
  is converged wholesale.
- A short **seed list** (`generatedSeed` in
  `ryoku/cli/internal/updater/materialize.go`: `fastfetch/config.jsonc`,
  `kitty/current-theme.conf`, the ghostty and nvim starting points, plus every
  provider's per-machine files from `wm.ConfigSeeds`, e.g. `hypr/monitors.lua`
  and `niri/monitors_user.kdl`) is copied only when absent, never clobbered:
  per-machine or user-owned state an update must keep, for every installed
  compositor, not just the active one.
- The user overlay (`~/.config/ryoku/user_edits`, mirroring `~/.config`) is laid
  on top last, so a file there wins at its mirrored path; see below. Anything the
  package never ships (`hypr/user.lua`, `kitty/user.conf`, a forked module) is
  left alone regardless.

So the QML and the `Config.qml` defaults reach users on every update. A **new**
`shell.json` key is safe: the user's file lacks it, and the shell reads the new
`Config.qml` default.

## user_edits: your edits, kept apart

Ryoku-owned config and user edits live in separate trees, so an update refreshes
the base freely while your edits stand. The base is the restore point; the
overlay is yours.

- **base** `/usr/share/ryoku/config` (the checkout on a dev box): pristine,
  re-laid in full on every update, so every fix and addition lands first.
- **user_edits** `~/.config/ryoku/user_edits`, mirroring `~/.config`, sparse:
  only what you changed. `materialize` overlays it last, so a file here wins at
  its mirrored path. Empty means pure base and the overlay is a no-op.

Two ways to override, neither of which blocks a fix:

- **Overlay (default).** The tool's own last-wins include: Hyprland loads the
  base modules, then `settings.lua` and `user.lua` last; kitty `globinclude`s
  `user.conf`. The base loads underneath, so a new upstream keybind still arrives
  while your file wins on what it sets.
- **Fork (opt-in).** A whole copy of a shipped file shadows the base one. You own
  it now, so an upstream fix to that file will not reach you automatically. Your
  forks are the files you see in the overlay; `ryoku reset <path>` takes the new
  base.

Ryoku Settings writes its generated `hypr/settings.lua` and `hypr/rebinds.lua`
into the overlay (authored under `user_edits`, reflected live). Its other state
(bar, colours, launcher, device lighting) it keeps under `~/.config/ryoku`,
GUI-managed and update-safe: the package ships no file there, so `materialize`
never clobbers or prunes it and a keyboard keeps the look you gave it across an
update. `ryoku reset` drops an override; `ryoku recovery` is the last
resort, wiping the overlay and that state back to shipped defaults.

## doctor: converging what materialize can't

`ryoku doctor` runs convergent reconcilers for the stateful drift materialize
can't state declaratively (disk, boot, session, and the user-owned
`~/.config/ryoku/*.json` materialize never rewrites). Reconcilers stand in for a
migration ledger: each is idempotent and safe on every update, and is retired
once every supported install has run it. `reconcileShellConfig` migrates a stale
`shell.json` (drops retired keys, revives the bar, clamps geometry).
`reconcileLauncherLocalFrostDefault` moves only the launcher's retired shipped
`bgBlur: 12` to the new 2 px local-frost default, then records a marker so a
later deliberate 12 remains a user choice.
`reconcileUserEdits` seeds the how-to guide and, for boxes upgraded from the
retired adopt step, moves the tool's own user files (`hypr/user.lua`,
`hypr/monitors_user.lua`, `kitty/user.conf`) back OUT of the overlay. Those are
edited in place; a frozen overlay copy of one used to be re-laid over the live
file on every update, wiping edits made afterward. Idempotent.
`reconcileMimeDefaults` clears the default-app map an older release froze into
`~/.config/mimeapps.list`: entries that only copy Ryoku's shipped values are
dropped (the file goes if that is all it held), and anything the user chose
stays. Ryoku's map ships to `/usr/share/applications/mimeapps.list` now, the
bottom of the XDG mimeapps chain, so it sets the defaults without ever
outranking a user's pick.
`reconcileShellInstances` clears a desktop that is running twice: a shell surface
orphaned by a daemon that was killed keeps drawing, and Quickshell allows a second
instance of one config, so the replacement draws over it. It keeps the instance
the supervising daemon started and stops the rest.
`reconcileShellLoad` gets a black screen back. Every surface is one Quickshell
instance, so a single QML file that cannot load takes the whole desktop, at login
and after an update alike. It reads the load failure from the shell daemon's
surface log (or loads the config once when there is no log), scopes the repair to
the module the loader blamed, moves a user override that breaks the desktop aside
as `.broken`, puts back every shipped file the live tree no longer matches, and
restarts the shell. When the shipped file is itself at fault it says so and names
`ryoku update` and `ryoku rollback`, the two things that help.
`reconcileWmPlugins` keeps a compositor's enabled plugins loading across a
compositor bump: a plugin is ABI-locked to the exact build and every copy Ryoku
builds carries an ABI receipt, so after an update it rebuilds each enabled
plugin whose receipts no longer match the installed headers through the provider
(`ryoku-hub desktop plugins rebuild --stale`, the Plugins page's builder) before
the next login, and names the toolchain to install when a box has none. Gated on
`CapPlugins`, so a compositor with no plugin system (niri) is a no-op. See
`docs/hyprland-plugins.md`.
`reconcileManifest` converges the box's package set to the release's control
manifest. It reads the channel's `manifest.json`, diffs it against the baseline
the box last converged to (saved with the names installed at that moment), and
installs what the release wants that this box never received, which is what
makes a package added to a set reach every box on the next update without a hard
depend, and heals a box that has been missing one all along. A name present at
the baseline and gone now was deleted by the user and stays gone; a name the
release retired is reported, never uninstalled. The deliver-once apps stay
`reconcileShippedApps'` lane, so one update never runs two transactions over the
same names. Best-effort: a box with no mirror or no network reports what did not
land and the update moves on. `ryoku verify` answers the same diff read-only, so
two machines can be compared line by line. This reconciler is the Arch build's: it
reports `ok` on a box without pacman, where package dependencies carry the set.

## Two compositors

A box can have both compositors installed and switch between them. Update, doctor
and recovery reach the compositor only through the seam (`ryoku/wm/`), so none of
them names one. Update and doctor keep the inactive compositor's config
untouched; recovery deliberately resets both when both are installed.

- **`ryoku update`** re-lays the base config, then reloads the active compositor
  through `wm.Open()` (`update.go` `pauseConfigAutoreload`/`reloadConfig` call
  `Act(ActionConfigReload)`; a compositor that watches its own file no-ops the
  reload). The seed list folds in every provider's per-machine files from
  `wm.ConfigSeeds` (`ryoku/cli/internal/updater/materialize.go`), so an update
  while niri is active never clobbers or prunes `hypr/*` seeds, and the reverse.
- **`ryoku doctor`** repairs compositor state through the seam and only for the
  running provider: it points xdg-desktop-portal at that provider's
  `Caps.PortalBackend` (`reconcilePortalRouting`), rebuilds stale window manager
  plugins only when the provider declares `CapPlugins` (`reconcileWmPlugins`; a
  compositor with no plugin system reports no plugin support), and writes the
  overlay how-to guide against the active provider's own config files
  (`reconcileUserEdits`, so it names `niri/user.kdl` on niri and `hypr/` paths on
  Hyprland). It does not touch the inactive compositor.
- **`ryoku recovery`** clears the `user_edits` overlay and the neutral Hub
  stores, then removes every path that `ryoku wm reset-paths` prints: each
  provider's generated config plus its hand-edit files (`wm.ResetPaths` over
  `wm.Providers`), for both compositors when both are installed. It runs that
  command from the freshly fetched checkout first (`go run . wm reset-paths`), so
  a broken installed build cannot skew the list, then redeploys the shipped
  defaults. The per-machine seeds (monitors, gpu, keyboard) and saved rices are
  not in that set and survive. `--no-packages` skips dnf; it refuses on a
  machine that is not Ryoku.
- **Switching** (`ryoku wm use <name> [--keep-previous|--remove-previous]`)
  installs the target's package; removing the old compositor reclaims its
  packages. `wm.Reclaim` computes the free set from the outgoing provider's
  `Caps.Packages`, and the package and byte counts shown come from the rpm
  removal plan, re-checked immediately before the transaction. Full switch
  contract in `docs/compositors.md`.

## Publishing: the COPR channel

A packaged Fedora box takes its Ryoku set from one repository: the COPR project
`itskontra/ryoku`, configured as `/etc/yum.repos.d/RyokuCOPR.repo` (repository
ID `RyokuCOPR`, channel name `copr`). There is no stable/testing split and no
frozen release directory: the channel rolls, and a release is a named point on
it.

Every push to `main` runs `publish-copr.yml`, which prepares the SRPMs once,
rebuilds them in clean Mock roots, installs the result on both compositors with
DNF5 and DNF4, submits the same SRPMs to COPR, verifies the returned RPMs
against the pinned COPR key, installs those unchanged RPMs again, and only then
regenerates the repository. A run that fails any gate publishes nothing.
Details and the publisher setup are in `release/rpm/README.md`.

Each build carries a strictly increasing package version (`0.<commit count>`,
with the workflow run number as the RPM Release), and the `ryoku-desktop`
package writes `/etc/ryoku-release` (`RELEASE=`, `NAME=`, `CHANNEL=`,
`VERSION=`, `COMMIT=`, `DATE=`) so a box can say what it runs.

On the box:

- `ryoku update` refreshes `RyokuCOPR` and runs `dnf distro-sync` restricted to
  that repository, so only the Ryoku set moves.
- `ryoku track copr` (or `main`) points `RyokuCOPR` back at the COPR channel;
  stable, testing and release tags are refused.
- `ryoku rollback` lists the system snapshots. COPR keeps a limited package
  history, so there is no rollback to a release tag; DNF can downgrade only to
  versions COPR still retains.
- `ryoku version` prints `RELEASE=`: the release tag on a release build, the
  tag plus the commits past it otherwise.

`ryoku track main --source` is the developer path: it builds and tracks a git
checkout (`~/ryoku-fedora`) instead of packages (see `docs/development.md`).

### Cutting a release

The annotated `v<X.Y.Z>` tag is the only human version Ryoku has; there is no
version file. The RPM `Version` is `0.<commit count>` (`prepare-srpms.sh`), the
one number dnf orders by, and never the release name. `/etc/ryoku-release`
(`RELEASE=`) carries `git describe` of the build: `v0.73.0` on a release,
`v0.73.0-5-gabc1234` past it, and `ryoku version` prints the same on a checkout.

Bump the minor for a release carrying a `Note: New` or `Note: Removed`, the
patch otherwise; `-rc.N` marks a candidate. Releases below 1.0 and candidates
are GitHub prereleases. Release when there is something to announce or the ISO
needs a refresh: `main` already ships continuously.

From a clean `main` at `origin/main`, with CI green on that commit:

```sh
bin/ryoku-release 0.73.0
```

It checks the tree, the tag, the version order and CI, prints the notes the
release will carry, and pushes the tag on confirmation. The tag then runs:

- `publish-copr.yml`: rebuilds the tagged commit under a higher RPM revision so
  boxes update onto the build whose `RELEASE=` names the release. The
  `fedora-publish` environment must allow `v*` tags to deploy.
- `build-fedora-iso.yml`, called by `publish-copr.yml` once that rebuild is
  published: waits for COPR to serve it, builds the ISO, and creates
  the GitHub release titled `Ryoku <CODENAME> <version>` with the notes
  `bin/ryoku-release-notes` harvests from the `Note:` trailers since the
  previous tag, then attaches the ISO and its checksum.

### Release names

Every release line has a name from the creation stories Ryoku draws on (the
Kojiki and the Theogony), in the order those stories tell them; `CODENAME`
holds the current one. The name changes when a line begins (the pre-1.0 line is Onogoro, the first
island; 1.0 is Amaterasu) and every release inside the line keeps it. It
travels with the release: `prepare-srpms.sh` writes it into `release.json` and
the ryoku-desktop package into `/etc/ryoku-release` (`NAME=`),
`build-fedora-iso.yml` titles the GitHub release with it, and a box shows it in `ryoku version --pretty` (which
fastfetch's OS line uses), `ryoku status`, `ryoku rollback`, the update
island (when the channel serves the next line) and the Hub's Updates page.

## The contract

- **Ryoku moves only what Ryoku publishes.** `ryoku update` upgrades the
  installed Ryoku packages by name and nothing else: no system upgrade, no
  kernel, no exclude list to maintain. Anything Ryoku needs from the base
  system belongs in a package dependency, where dnf resolves it, not in a
  transaction that quietly upgrades the machine. A user must never have to
  choose between a Ryoku fix and their own upgrade schedule, and nothing may
  block `sudo dnf upgrade` (the one exception is the signed NVIDIA driver
  above, which waits for its module).
- **What a surface shows about the system must be read from the system.** No
  kernel, variant, or OS name is hardcoded into a menu, a default, or a report:
  boot entries come from the installed kernels
  (`/usr/lib/modules/*/pkgbase`), the boot default from
  `/etc/ryoku/default-kernel` (what the install chose) then the running kernel,
  and an entry whose image is not on the boot partition is removed. A box must
  never be offered a kernel it does not have.
- **A user-facing config file must be delivered by a path a user runs**: shipped
  in a package (then materialized) or seeded by the installer. A file only
  `deploy.sh` lays, or one no path lays, reaches no user. `ryoku-dev-verify-delivery`
  fails the commit on such an orphan.
- **A package the release is made of must reach every box on update.** On
  Fedora that means a package dependency: the `ryoku-desktop` spec in
  `release/rpm/` pins its components, so a package added there arrives with the
  next `ryoku update`. The control-manifest reconciler (`reconcileManifest`) is
  the Arch build's mechanism and reports `ok` on a box without pacman.
- **A removed or renamed `shell.json` key, or a changed default that must reach
  existing users, needs a `doctor` reconciler** (materialize never edits a user's
  `shell.json`). An additive key needs nothing.
- **Never ship into a path the user's own tools write, and never write a tool's
  output into a shipped path.** `materialize` clobbers every shipped file, so
  laying Ryoku's defaults where an app writes the user's choice resets that
  choice on each update: `~/.config/mimeapps.list` did exactly that to default
  apps. Ship such defaults one layer down where the format provides one
  (`/usr/share/applications/mimeapps.list` for mime defaults), or make the file
  a `generatedSeed` if it has no layering. The same rule read the other way:
  a rice used to copy its emblem over the shipped
  `fastfetch/fastfetch-emblem.png`, and every update put the brand mark back.
  Anything Ryoku writes on the user's behalf (an imported logo, a rice asset)
  goes to a user-owned name the package never ships (`fastfetch/ryoku-logo.*`).
- **A user override belongs in `~/.config/ryoku/user_edits`, never in a shipped
  path.** The base still ships every file (the delivery check stays green) and
  the overlay wins on top. A whole-file fork opts out of upstream fixes for that
  one file, so prefer an overlay for anything additive. An edit made to a
  shipped file in place is not lost either: `materialize` notices bytes that
  match neither what it laid last time nor what it ships now, copies them into
  the overlay as a fork, lays the base, and lists the files it kept. Hyprland
  additions (rules, binds) belong in `hypr/user.lua` or the Hub, which are
  never re-laid.
- **Everything a user runs must converge on update, wherever it lives.**
  `materialize` covers `~/.config`; a payload installed elsewhere (the lock
  bundle under `~/.local/share`, the SDDM greeter skin under
  `/usr/share/sddm/themes`) needs a `doctor` reconciler that compares content
  with the shipped copy and re-lays it on drift, not one that only checks it
  exists. An install-once path silently pins every existing box to the release
  it was installed with: the lock shipped fixes for weeks that no updated box
  ever received.
- **A system path a package owns may exist unowned first, and the update adopts
  it.** The ISO installer and `ryoku/shell/deploy.sh` seed some paths before a
  package owns them: the privileged helpers and their polkit rules, the Plymouth
  theme, the shipped boot configs, and the logind lid-switch drop-in
  `/etc/systemd/logind.conf.d/10-ryoku-lid.conf`. An unowned copy collides with
  the package on the next transaction ("exists in filesystem") and aborts the
  whole atomic `-Syu`, so `ryoku update` passes `--overwrite` for the seeded
  globs (`updater.ryokuOverwriteGlob`, fed by `unownedFiles`) and the doctor
  clears the same paths on a box already wedged
  (`reconcileConflictingRyokuFiles`, `ryokuSystemGlobs`). Either way the package
  adopts the path and later updates own it normally; `deploy.sh` keeps its own
  copy of the list (`_rovw`) in sync.
- **Generated power policy and long-running helpers must be adopted as one live
  transaction.** `hypridle.conf` is rendered state, not materialized payload,
  and a running shell-script daemon keeps executing its old file after the
  package replaces it. The desktop RPM's scriptlets (`%pre`/`%preun` then
  `%posttrans`/`%postun`) preserve the pre-transaction executor and
  synchronously adopt every live Ryoku user after install, upgrade, or
  removal. The shared lifecycle helper selects one confirmed active Ryoku
  session per user, watches login1 activity/logout, and keeps its
  watcher retryable through D-Bus outages. Package stage two, login, and
  checkout deploy use the same durable login1 block while activating logind's
  sessionless fallback, proving old owners stopped, requiring the compositor
  bindings to reload, and replacing shell, idle, clamshell and wallpaper
  owners. Doctor instead stages a complete qylock repair under the generation
  guard for the next managed shell activation. Live-cutover protection releases
  only after the new shell reports its inhibitor state and service ownership is
  verified; failure remains blocked until a successful retry or reboot. When
  the package adoption did not complete, stage two invokes the all-session
  helper synchronously. A `--no-reload` checkout deploy stages the drop-in
  without changing live logind, so the running session never mixes old and new
  halves.
- **One master per setting.** Two stores that both claim a value drift, and the
  next sync of either undoes the other: the colour master is `shell.json`
  `theme.theme` (the daemon shadows it into `theme.json` `followWallpaper` on
  every load), so the Hub's scheme cards and a rice select the theme through
  `ryoku-shell theme` instead of writing the shadow. A new setting gets one
  writer; every other surface reads.
- **Shipped QML must load, not just exist.** One file that cannot instantiate
  blanks its whole surface (a Hub page, a shell root), and Quickshell reports it
  only in the instance log. `bin/ryoku-dev-lint-qml` fails on the qmllint
  classes that mean "will not load", resolved against the installed modules;
  the publish gate runs it over the materialized tree.
- **Login restarts the session daemons.** The user manager can outlive a
  session (linger, a relogin after a compositor crash) and still hold a
  `ryoku-shell` bound to the dead compositor; `start` then does nothing and the
  login lands on bare Hyprland. The autostart reloads units, clears a start
  limit, and `restart`s the shell and wallpaper daemons every session.
- **A change reaches users when it lands on `main`**: every push publishes to
  the COPR channel. A release tag names a point on that stream; it gates
  nothing.
- **Displayed English must be wrapped where it is displayed, or it never
  translates.** `I18n.tr("...")` in QML, a Hub schema `label`/`desc`, `i18n.T`
  in Go, `log 'fmt %s'` in the installer's shell. The `i18n` workflow extracts
  only what is wrapped, so an unwrapped string ships English in all 35
  languages and no later pass finds it. Keep a sentence whole with `%1`/`%s`
  placeholders rather than concatenating fragments: a fragment freezes the word
  order to English. `docs/i18n.md` has the rules; a new language is one row in
  `ryoku/i18n/langs.json` and nothing else.

## Checks

- `bin/ryoku-dev-verify-delivery` flags orphan configs (hard fail). It runs in `pre-commit`, `post-commit`, and the Delivery check
  workflow.
- The install-test workflow builds the ISO and runs a real, unattended install in
  a VM, then verifies the desktop comes up, so a broken install or a missing
  package is caught before a user hits it.
- The publish (`publish-copr.yml`) installs the exact RPMs COPR built, with
  the COPR key verified, on both compositors with DNF5 and DNF4
  (`installation/tests/fedora-rpm.sh`) before it regenerates the repository.
  What was tested is byte-for-byte what ships; a run that fails the gate
  publishes nothing.
- `bin/ryoku-dev-lint-qml <config-root>...` fails on QML that cannot load. The
  publish gate (`installation/tests/container-install.sh`) runs it over the
  materialized shell and Hub trees against the installed Qt modules, the same
  import path a user's session resolves; run it on a dev box after
  `ryoku deploy` before pushing a QML change.
