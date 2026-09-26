# Kernels

Ryoku Fedora boots Fedora's stock `kernel` package. Ryoku publishes no kernel of
its own and never decides when the kernel moves: `sudo dnf upgrade` does that,
when you run it, and `ryoku update` leaves it alone (see `docs/updates.md`).

## How the kernel moves

- **Updates.** A new kernel arrives from the Fedora repositories with
  `sudo dnf upgrade`, or with `ryoku update --system`, which runs the same lane.
  `ryoku update` and `ryoku status` report how many system packages, the kernel
  among them, are waiting.
- **Fallback.** dnf keeps the previous kernels installed (`installonly_limit`), so
  a kernel that misbehaves is one GRUB menu entry away from the one that worked.
- **Boot entries.** Fedora writes a BootLoaderSpec entry per installed kernel;
  kernel arguments are changed through `grubby`, never by editing the entries by
  hand.

## The signed NVIDIA driver

The one case where Ryoku holds a kernel back is opt-in. A box running the
Secure Boot signed NVIDIA driver (`ryoku-nvidia`) cannot boot a kernel newer than
the newest one that driver has a signed module for, so the package conflicts
with those kernels and `dnf upgrade` skips them until the module is published.
The kernel still moves only when you run the upgrade. Details in
`system/hardware/README.md`.

## The CachyOS kernel

The Arch build offers the CachyOS kernel as a one-click Extras bundle
(`cachyos-kernel`). It adds a pacman repository and has no Fedora equivalent, so
`ryostore-install` refuses it on Fedora ("the CachyOS kernel bundle is not
supported on Fedora"). Installing a third-party kernel on Fedora by hand is
possible, but it is outside what Ryoku ships or supports.
