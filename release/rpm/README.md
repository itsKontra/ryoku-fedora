# Ryoku Fedora Packaging & RPM Repository

This directory contains the RPM packaging specifications and repository automation for deploying Ryoku to Fedora Linux.

## Structure

- `*.spec`: RPM spec files for Ryoku's core packages (`ryoku-desktop`, `ryoku-shell`, `ryoku-hub`, `ryoku-rashin`, `ryogami`, `ryotunes`, `ryostore`, `ryovm`, `sddm-theme-ryoku`).
- `build-rpm-repo.sh`: Shell script to compile all specs using `rpmbuild` and generate a local or hosted RPM repository via `createrepo_c`.

## Prerequisites on Fedora

```bash
sudo dnf install rpm-build createrepo_c golang wayland-devel ffmpeg-free-devel
```

## Building Packages Locally

Run the repo builder from this directory:

```bash
./build-rpm-repo.sh
```

This will produce the RPM binaries under `out/` and index them with `createrepo_c`.

## COPR Build & Distribution

To build and host packages via Fedora COPR:
1. Create a project in [Fedora COPR](https://copr.fedorainfracloud.org/) named `ryoku`.
2. Submit builds using `copr-cli build ryoku <specfile-or-srpm>`.
3. Users can enable the repository using:
   ```bash
   sudo dnf copr enable neur0map/ryoku
   sudo dnf install ryoku-desktop
   ```
