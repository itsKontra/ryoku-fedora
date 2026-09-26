# Merging Ryoku upstream into Fedora

Use this playbook whenever upstream Ryoku (`unstable-dev` on
https://github.com/neur0map/ryoku-arch) needs to be brought into this Fedora
port. It keeps the desktop and product changes from Ryoku while keeping
Fedora as the only delivery, installer, package-manager, and boot-stack
implementation.

Every sync is a regular merge commit on a PR branch, and the PR is merged with
a merge commit. That records the upstream parent, so the next sync contains
only newer upstream commits. Do not squash it and do not cherry-pick the whole
range: either leaves the branches unrelated, and the next sync replays
everything already taken as conflicts. PR #23 was squashed that way; the merge
that records its upstream tip (`3fa2b4ed`) as a parent repaired it without
changing a file. Picking single commits does not fit this upstream either:
many upstream commits touch shared code and Arch delivery at once.

`bin/ryoku-dev-sync-upstream` does the mechanical half of this playbook. Its
path lists are the machine-readable copy of the tables below; change both
together.

## Scope rule

Classify every upstream change by the interface it affects, rather than by its
commit title. A desktop feature may need an RPM dependency, an installer
package, a DNF implementation, and a migration before it is usable on Fedora.
An upstream feature is complete only when all applicable Fedora layers agree.

| Change type | Fedora action |
| --- | --- |
| QML, Go product logic, shared UI, translations, assets, and compositor-neutral code | Merge, then add or adjust RPM dependencies and delivery checks when the feature has a new runtime need. |
| Hyprland/niri configuration or provider code | Merge the feature through `ryoku/wm/`; keep provider configuration in its native language and verify both Fedora provider RPMs. |
| Shell script or hardware helper | Port package-manager calls, executable names, filesystem locations, services, PAM, SELinux, or boot tooling before retaining it. |
| User config shape, generated config, or persistent store | Merge the change and add an idempotent `ryoku doctor` reconciler when an existing Fedora installation must change state. |
| Documentation, workflow, package recipe, installer, update channel, or test | Keep it only after adapting it to DNF/RPM, Anaconda, COPR/release repositories, and the Fedora CI matrix. |

## Keep Fedora-owned paths

These paths are the port's source of truth. Resolve a merge conflict in favour
of `main`, then manually copy only an upstream change that has been reviewed
and translated for Fedora.

| Paths | Reason |
| --- | --- |
| `installation/fedora/`, `installation/tests/fedora-*`, `installation/tests/fedora-*.py`, `installation/README.md` | Anaconda image composition, first boot, provisioning, signatures, and Fedora installation tests. |
| `release/rpm/` | RPM specs, payload staging, SRPM/COPR publication, release repository layout, and RPM verification. |
| `.github/workflows/build-fedora-iso.yml`, `fedora-firstboot.yml`, `fedora-iso-vm.yml`, `fedora-rpm.yml`, `publish-copr.yml` | The Fedora build, install, and publishing pipeline. |
| `ryoku/cli/internal/sys/rpm.go` and its tests | DNF/RPM channel and repository behaviour. |
| Fedora-specific doctor, updater, installer, recovery, and tracking code | DNF package ownership, RPM repositories, dracut/GRUB, and the Fedora migration contract differ from Arch. |
| `ryoku-shell-installer/fedora_repo.go` and Fedora branches in the installer | Repository setup and package-mode installation on Fedora. |
| Fedora package declarations and dependency checks | Fedora package names and available COPRs are authoritative here. |

Keep upstream improvements to shared callers of these paths, but implement the
Fedora equivalent beside the existing DNF/RPM code. Never make a Fedora path
fall back silently to Pacman, AUR, Arch repositories, or a source build.

## Discard Arch- and CachyOS-only content

Do not restore these upstream implementations or their related documentation,
CI, tests, keys, and release metadata. They are intentionally absent from the
port and must remain absent after every merge.

| Upstream paths or concepts | Fedora replacement |
| --- | --- |
| `installation/backend/`, `installation/iso/`, `installation/tui/`, Arch installer tests | `installation/fedora/` and `installation/tests/fedora-*`. |
| `release/packages/` PKGBUILDs, `release/repo/`, `release/gradle-offline/`, Arch package keys | `release/rpm/` specs, staged payload recipes, SRPM/COPR publishing, and RPM keys. |
| Pacman, yay, AUR, CachyOS repositories, `makepkg`, `pacstrap`, `archiso`, `mkinitcpio`, Limine, Snapper | DNF/RPM, Anaconda/Kickstart, dracut, GRUB, and the Fedora release/update implementation. |
| `system/boot/` Arch boot assets and CachyOS-specific system/package policy | Fedora boot and package policy already represented by the installer and RPM payloads. |
| Arch release/channel workflows and Arch-only CI | Fedora workflows and the COPR/release-repository checks. |

An upstream directory omitted above is not automatically portable. Search its
commands and dependencies before keeping it. Treat `pacman`, `yay`, `aur`,
`CachyOS`, `PKGBUILD`, `makepkg`, `pacstrap`, `archiso`, `mkinitcpio`, `limine`,
and `snapper` in a proposed Fedora diff as blockers that need an explicit
translation or removal.

## Merge and adapt shared work

Bring these areas forward by default, with the listed Fedora checks.

| Area | Merge work | Fedora follow-through |
| --- | --- | --- |
| `ryoku/shell/`, `ryoku/ui/`, `ryoku/hub/`, `ryoku/apps/`, `ryoku/i18n/` | Product UI, shell daemon, wallpaper, store, settings, translations, tests, and compiled shader pairs. | Add missing runtime packages to the owning RPM spec; stage every new file; run QML/Go tests and test the surface on Fedora. |
| `ryoku/wm/`, `ryoku/hyprland/`, `ryoku/niri/` | Provider seam, capabilities, settings, keybinds, display, plugin, and config changes. | Update `ryoku-desktop-hyprland.spec` and `ryoku-desktop-niri.spec` payloads/dependencies; exercise each provider on Fedora. |
| `ryoku/lockscreen/`, SDDM, portals, polkit, and systemd user units | Behavioural and security fixes. | Check Fedora PAM service names, systemd unit locations, portal packages, file labels, and SELinux behaviour. |
| `system/hardware/` and `system/containers/` | Hardware and helper fixes that are distribution-neutral in behaviour. | Translate tools/packages and recheck udev, polkit, systemd, dracut, and DNF ownership. Keep only helpers the RPM payload actually ships. |
| `ryoku/cli/`, updater, doctor, `ryoku-shell-installer/`, and `ryoku/rashin/` | Shared product behaviour, migrations, developer tooling, and tests. | Preserve RPM/DNF branches; implement equivalent channel, package query, recovery, and cleanup semantics; add tests for the Fedora path. |
| Generic docs and CI | User-visible behaviour, developer instructions, and portable tests. | Rewrite Arch-specific commands and package names; retain generic workflow checks only when their dependencies exist in Fedora CI. |

New upstream packages must be declared in the RPM spec that owns the feature
and, when needed for an ISO or first boot, in the Fedora installer package set.
Do not revive `system/packages/*.packages` as an installation source; the RPM
specs and `installation/fedora/packages.list` define Fedora delivery.

## Repeatable PR procedure

1. Start from a clean, up-to-date `main`. Preserve unrelated local work before
   starting: `git status --short`, `git fetch origin --prune`, then
   `git switch main` and `git pull --ff-only`.
2. Review what is pending with `bin/ryoku-dev-sync-upstream --report`. It adds
   the `upstream` remote when missing, fetches `unstable-dev`, and lists every
   new upstream commit as `auto` (version bumps and translation runs),
   `discard` (only paths on the discard list), `mixed`, or `portable`.
3. Create `merge/fedora-upstream-YYYY-MM-DD` from `main` and run
   `bin/ryoku-dev-sync-upstream` on it (`--ref <sha>` pins a candidate). It
   begins one ancestry-preserving `git merge --no-ff --no-commit`, then:
   - removes upstream additions and edits under the discard list;
   - keeps files `main` deleted deleted, and lists them;
   - keeps `main` for conflicted Fedora-owned paths, and lists them so the
     upstream change can be ported by hand;
   - merges conflicted i18n catalogs key by key
     (`ryoku/i18n/tools/merge3.py`);
   - refreshes the vendored `ryoku-wm` copies from `ryoku/wm` once the seam
     is resolved;
   - prints the conflicts left, the files whose staged diff names an Arch
     term, and the upstream range for the PR record.

   Do not use `-X ours`, `-s ours`, a blanket checkout of one side, or a
   squashed import for a sync. Those hide conflicts that may contain product
   work or reintroduce an Arch implementation. (`-s ours` is only right for
   recording a range whose content already reached `main`, as with #23.)
4. Resolve the remaining conflicts by hand. For each source conflict, preserve
   the upstream behaviour and retain the Fedora package, installer, update,
   and service contracts. Read each "kept main" path's upstream diff and port
   what applies. Check each "kept deleted" path for a portable fix that
   belongs somewhere else in the Fedora tree.
5. Search the staged result for distribution leakage and for untracked delivery
   work. The script lists files with a match; review `git diff --cached
   --name-status`, then read the matches with an exact search such as:

   ```bash
   git diff --cached -G 'pacman|yay|AUR|CachyOS|PKGBUILD|makepkg|pacstrap|archiso|mkinitcpio|limine|snapper' -- . ':!docs/archived/**'
   ```

   Every remaining match needs a documented reason. Review added executable
   files, systemd units, PAM files, desktop entries, udev/polkit rules, shaders,
   Go module/vendor changes, and all RPM payload file lists.
6. Adapt delivery before calling the merge complete. Update the affected RPM
   spec and `release/rpm/payload/` script, Fedora installer package list or
   first-boot provisioning, repository/dependency configuration, migrations,
   documentation, and tests. Include regenerated i18n catalogs and `.qsb`
   files with their source changes.
   For every new or changed runtime package, explicitly verify both delivery
   layers: add it to the owning RPM spec when the packaged feature needs it,
   and add it to `installation/fedora/packages.list` when it must be present
   on the ISO or during offline installation. A package is not delivered until
   both applicable checks pass.
   Do not port NVIDIA-specific changes into Fedora when the Fedora image does
   not configure or deliver NVIDIA drivers. Leave those changes out and record
   them as deferred rather than adding a non-functional Fedora path.
   Review every changed path under `ryoku/`, including the shell, UI, Hub,
   apps, compositor seam, providers, translations, and assets. Verify that
   each feature and new UI surface is included in the Fedora RPM payload and
   installer path and works with the relevant Hyprland and niri provider. Add
   a concise summary of the successfully ported `ryoku/*` features to the PR
   description. If any `ryoku/*` feature cannot be ported, add a PR comment
   identifying the missing feature, the reason, and the reviewer decision
   needed before merge.
7. Commit the resolved merge with the repository hooks enabled, push the branch,
   and open a PR into `main`. The PR description names the upstream start/end
   revisions, summarizes discarded Arch work, lists Fedora adaptations, and
   includes the ported `ryoku/*` feature summary and any unresolved-port
   reviewer comments. Merge the PR with a merge commit so Git retains the
   upstream parent.

If the merge is not viable, use `git merge --abort`; do not leave a partial
resolution or replace `main` with the upstream branch.

## Required validation

Run the focused checks for every changed area, then the Fedora delivery checks
that prove the feature reaches a user. At minimum, run `git diff --check`, the
relevant Go tests, shell syntax checks, and QML lint/tests where available.

For any RPM, installer, updater, provider, portal, lockscreen, service, or
hardware change, run the relevant scripts in `installation/tests/` and
`release/rpm/tests/`. Build all affected RPMs in the Fedora test path and test a
fresh install plus an update from the prior package set. Test Hyprland and niri
when a shared shell, compositor seam, package, or installer change can affect
both. Validate graphical changes in a Fedora VM with SELinux enforcing when
they touch login, locks, portals, hardware, or system services.

For NVIDIA-related upstream changes, explicitly verify whether Fedora configures
and delivers the required NVIDIA driver stack. If it does not, do not claim the
change is ported: omit the Fedora implementation and record it as deferred.
For every changed `ryoku/*` feature, verify source inclusion, RPM ownership,
ISO/install delivery, and the relevant compositor/provider behaviour. The PR
must summarize the features that passed this review and must contain a reviewer
comment for each feature that could not be ported.

The PR is ready only when the package owns every shipped file, no Arch delivery
path has returned, existing user overlays survive materialization, and a Fedora
system can receive the result through its normal RPM update path.

## PR record

Put this compact record in every update PR so the next merge has an audit trail:

```text
Upstream range: <previous upstream parent>..<candidate SHA>
Merge commit parent: <candidate SHA>
Discarded: <Arch/CachyOS paths or commits and why>
Fedora adaptations: <RPM/installer/updater/doctor/service changes>
Validation: <commands, Fedora version, providers, and VM evidence>
Deferred: <specific upstream work intentionally left for a follow-up>
```

Do not defer a dependency or migration required for an imported user-visible
feature. Either complete its Fedora delivery in the same PR or leave the
feature out of that merge.
