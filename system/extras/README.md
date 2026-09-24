# system/extras/

Fedora helpers for installing, removing, and reporting optional bundles from
Ryostore. They ship to `/usr/bin` with `ryoku-desktop`; nothing runs at boot.
Ryostore owns the catalogue, its cache, and script/plugin downloads.

## Helpers

- `ryostore-install` installs, removes, and reports bundle items. It reads
  definitions through `ryostore internal bundle` and publishes per-item state to
  `$XDG_RUNTIME_DIR/ryostore/<id>.json` (falling back to `/tmp`).
  `RYOSTORE_DRYRUN=1` prints actions without running installers, querying remote
  package metadata, or changing packages. Catalogue fetching and report writes
  still occur.
- `ryoku-pkg-add` installs packages through `dnf -y install`.
- `ryoku-pkg-remove` removes packages and unneeded dependencies through DNF.
  DNF can also remove dependent applications, so its transaction confirmation
  remains enabled in the store's terminal. Cancelling does not trigger retries.
- `ryoku-pkg-aur-add` is a compatibility entry point for older catalogue
  scripts. It forwards to `ryoku-pkg-add`; Fedora has no AUR build path.
- `ryoku-pkg-multilib` checks for an x86_64 host and DNF. Fedora carries i686
  packages in its normal repositories, so no separate multilib repository
  needs enabling. Package availability is checked when installing.
- `ryoku-cmd-present` checks whether a command is on `PATH`.

## Packages and prerequisites

Package items must name Fedora RPM packages or RPM capabilities available from
an enabled repository. Architecture-specific items use `name.i686` or
`name.x86_64`. Arch package names are not automatically translated. Packages
missing from enabled repositories are reported as failed while the remaining
items continue; they never fall back to source builds or automatically enable
third-party repositories. Configure any required RPM Fusion or COPR repository
before installing a bundle.

Status takes one local RPM snapshot, including names, architectures, and
provided capabilities. Install/remove query live RPM state, and removal resolves
capabilities to concrete `name.arch` owners. DNF repository queries check both
success and nonempty results, since an empty match can still exit successfully.
See the [DNF repoquery reference](https://dnf5.readthedocs.io/en/stable/commands/repoquery.8.html).

`"requires": ["multilib"]` checks i686 support. `"requires": ["gpu-lib32"]`
runs `ryoku-gpu-lib32`, which installs Fedora i686 graphics libraries and matches
NVIDIA libraries to the installed RPM Fusion driver branch. Failed prerequisites
abort installation. CachyOS kernel bundles are unsupported on Fedora and are
rejected before prerequisites run; the CachyOS repository helper was removed.

Script items run the installer supplied by the catalogue, which must itself
support Fedora. Plugin and file-manager script items use Ryostore's existing
install/remove paths.
