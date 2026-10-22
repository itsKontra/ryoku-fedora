# Fedora builds and signs the NVIDIA driver on each machine

On Fedora, Ryoku installs RPM Fusion's `akmod-nvidia`, which compiles the open
kernel modules on the machine and signs them with that machine's own module
signing key. Ryoku does not prebuild modules and sign them with a key it holds,
even though the Arch edition prefers prebuilt modules and the fork's
`ryoku-nvidia` COPR proved CI signing works on real hardware.

A Ryoku-held key would mean holding a key that every NVIDIA machine trusts, its
rotation and revocation, and a CI pipeline that publishes a module for every
kernel Fedora serves. That pipeline is the part that failed. Once the fork's
scheduler stopped, the `Conflicts: kernel-core` hold froze a box on an old
kernel without saying so. The hold also leaked: newer kernel subpackages were
installed anyway. Avoiding a compiler on the target saves nothing here, because
every Fedora desktop already ships `gcc`, `kernel-devel` and `dkms` for
`xpadneo-dkms`, and that module already needs a per-machine key of its own.

## Consequences

- A machine has exactly one module signing key, DKMS's `/var/lib/dkms/mok.key`.
  On NVIDIA machines akmods' key files are links to it, so xpadneo and NVIDIA
  need a single enrollment, and every Fedora machine with Secure Boot on goes
  through the same enrollment path. akmods builds and signs as its unprivileged
  `akmods` user, so wherever akmods is installed the key is `0640 root:akmods`.
- RPM Fusion free and nonfree are enabled on NVIDIA machines for the driver
  alone. Userspace follows RPM Fusion's stream without a versionlock, and
  RPM Fusion's `lspci` check picks the open modules. The tainted repo is never
  enabled.
- A kernel can arrive before its NVIDIA module. Ryoku holds no kernel back and
  never changes the boot default: akmods builds at kernel install and again at
  boot, and a kernel still without a module boots into a driver fallback on
  nouveau, which doctor reports and tries to heal by rebuilding.
- The private key stays on disk, readable by root and, on NVIDIA machines, the
  `akmods` group: the same exposure RPM Fusion's own akmods key has. A second
  key would not narrow it, since Secure Boot trusts every enrolled key equally.
  No disk encryption is required. Someone holding an unencrypted disk can
  already rewrite its root filesystem and initramfs.

See the wayfinder ticket "Which driver source and signing approach does Ryoku
adopt on Fedora?" (itsKontra/ryoku-fedora#197) and its research.
