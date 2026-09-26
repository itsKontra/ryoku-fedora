# Installation

Ryoku installs on mutable Fedora 44 x86_64 systems. This directory contains the
Fedora ISO compose, offline provisioning, first-boot setup, and their tests.

## Layout

- `fedora/`: the Anaconda/Kickstart ISO implementation. `build-iso.sh` composes
  the image, `build-repo.py` creates and validates the offline RPM closure,
  `provision-target.py` configures the installed target, and `firstboot.py`
  completes account and regional setup before graphical login.
- `tests/`: container and VM coverage for the Fedora repository, RPM set,
  provisioning, first boot, signatures, channels, ISO staging, and ISO boot.

The retired Arch installer, Bubble Tea disk TUI, shell backend, and archiso
profile are not part of the Fedora installation path.

## Install paths

- A fresh machine boots the Fedora ISO. Anaconda installs the offline RPM
  closure and the provisioner prepares Ryoku in the target root.
- An existing mutable Fedora installation uses `ryoku-shell-installer/`, which
  configures the Ryoku COPR and installs the selected compositor packages with
  DNF. It does not partition disks.

See [fedora/README.md](fedora/README.md) for build requirements, repository
contracts, compose commands, and required VM evidence. See
[tests/README.md](tests/README.md) for the focused verification commands.
