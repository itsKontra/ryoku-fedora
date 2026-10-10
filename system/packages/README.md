# system/packages/

Package lists inherited from upstream Ryoku, written in Arch package names. One
package per line; blank lines and lines starting with `#` are ignored.

They are not how Fedora installs Ryoku. The RPM specs in `release/rpm/` and
`installation/fedora/packages.list` define Fedora delivery (`MergeToFedora.md`).
Do not add a package here to ship it; declare it in the spec that owns the
feature.

## What still reads them

- `ryoku-shell-installer` reads `base.packages` and `dev.packages` and translates
  each name for the detected distribution (`ryoku-shell-installer/distro.go`).
  `ryoku recovery` resolves its dnf dependency list through it.
- `ryoku/cli/cmd/ryoku-manifest` reads `base`, `dev`, `hardware` and `aur` into
  the update manifest.
- `tests/shell-tool-availability.sh` checks that a tool the shell calls is
  listed here and is a hard `Requires:` of `ryoku-desktop`.

## The lists

- `base.packages` the set every machine gets: core system, networking, audio,
  the session, terminal apps, the shell stack, and fonts.
- `dev.packages` developer toolchains (Go, Node/npm, Rust, Python/pip, mise).
- `hardware.packages` per-profile microcode and GPU extras, in `[amd]`,
  `[intel]`, `[nvidia]` and `[vm]` sections.
- `aur.packages` upstream's AUR extras. Nothing on Fedora installs them.
