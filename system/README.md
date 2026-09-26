# system/

How an installed Ryoku machine is put together, separate from the desktop in
`ryoku/`.

## What's here

- `packages/` Package manifests used by the shell installer and delivery audits:
  `base.packages` for the desktop system, hardware and development lists, and
  distro translation metadata.
- `recovery/` What the machine does when the desktop cannot start: the `sddm`
  drop-in and units that fall back to a text login on tty1 with a recovery
  banner, and the kernel-install plugin that adds a "Ryoku console" boot entry
  per kernel. See "The console fallback" in `docs/updates.md`.
- `hardware/` Hardware setup. `gpu/` picks the most capable GPU and pins it for
  Hyprland, `display/` scales high-resolution screens, and `drivers/` installs the
  right packages per vendor. The GPU and monitor settings are written as Hyprland
  Lua drop-ins.
- `policy/` System authorization rules (polkit). `52-ryoku-timedate.rules` lets the
  active desktop user set the time zone without a password, so the world-map
  picker in Settings applies instantly.

## Networking and services

Networking is NetworkManager. SDDM and NetworkManager are installed through the
Fedora RPM/installer path; service policy lives with the Fedora provisioner and
RPM payloads rather than in a second installer backend.
