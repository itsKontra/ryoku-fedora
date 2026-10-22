# fedora/iso: the Ryoku Fedora installer ISO

This directory builds an installer ISO for the Fedora edition. It is a fork
feature: upstream retired its Fedora ISO, so everything for it stays in this
directory (plus `.github/workflows/build-iso-fedora.yml`) to keep rebases onto
upstream trivial. Do not spread it into shared files or the docs.

How it works: `build.sh` remasters the official Fedora Everything netinst with
`mkksiso`. Anaconda installs a minimal Fedora with the user's own disk,
account, keyboard and time zone choices; the kickstart then runs
`post/provision`, which repeats the shell installer's Fedora steps
(`ryoku-shell-installer/engine.go`) as root in the target: the signed repo at
repo.ryoku.dev and its COPRs, the package plan resolved by
`system/packages/resolve`, SDDM, snapper and per-account seeds. Nothing is
bundled offline. `ryoku materialize` lays each account's config at first login.

| Path | Job |
|---|---|
| `build.sh` | build the ISO (`--channel stable\|testing\|<tag>`, `--release N`) |
| `fetch-base.sh` | download and verify the Fedora netinst |
| `kickstart/ryoku.ks` | Anaconda recipe (`@RELEASE@`, `@VOLID@` filled by build.sh) |
| `anaconda/05-ryoku.conf` | Anaconda drop-in: Btrfs with `/.snapshots`, no root password page |
| `post/` | the kickstart's scripts: `check-admin`, `stage`, `provision` |
| `tests/check.sh` | static checks; `--online` also checks the kickstart's groups |
| `tests/vm-install.sh` | unattended install in QEMU, then checks the installed system |

When the shell installer's Fedora path changes upstream, mirror the change in
`post/provision`. Run locally:

```sh
iso=$(fedora/iso/fetch-base.sh --release 44 --dest fedora/iso/work/base)
sudo fedora/iso/build.sh --base-iso "$iso" --channel testing
fedora/iso/tests/vm-install.sh --iso fedora/iso/out/*.iso --work fedora/iso/work/vm --display gtk
```
