# RPM Fusion is enabled only for the NVIDIA driver, only on NVIDIA machines

On Fedora, Ryoku enables RPM Fusion free and nonfree when, and only when, it
installs the NVIDIA driver: the shell installer and the ISO do it on a machine
whose NVIDIA GPU passes the generation check, and doctor offers the same
command. A machine without a supported NVIDIA GPU never gets RPM Fusion from
Ryoku, and Ryoku installs nothing else from it, even where a package (Steam,
codecs, other akmods) would be convenient.

`akmod-nvidia` and its userspace exist only in RPM Fusion nonfree, which needs
free. Enabling a third-party repository is a trust decision on the user's
behalf, so Ryoku makes it only where the driver needs it, and keeps it to the
packages that need it.

## Consequences

- `ryoku-host kmod install-driver` is the one place that enables RPM Fusion. It
  installs RPM Fusion's own release packages for the running Fedora version, so
  the repository files and keys are the ones RPM Fusion ships and follow its
  updates.
- Before installing a release package, Ryoku checks the key inside it against
  fingerprints pinned in `ryoku/cli/internal/kmod/rpmfusion.go`, then checks
  that the package is signed by that key, the same pin-then-trust pattern the
  installers use for Ryoku's own release key. When RPM Fusion rotates its keys,
  the pins are updated in that file.
- The tainted repositories are never enabled. No package is version-locked:
  the akmod and its userspace follow RPM Fusion together, and kernels are never
  held back for them (ADR 0001).
- A user who enabled RPM Fusion themselves keeps their setup; Ryoku skips the
  step when both release packages are already installed.

See ADR 0001 and the spec "NVIDIA driver on Fedora with Secure Boot"
(itsKontra/ryoku-fedora#204).
