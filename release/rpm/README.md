# Fedora RPM delivery

The port is experimental. Container builds verify package construction and
installation; graphical sessions and SELinux require the VM checks below.
Nothing in this directory implies that a public RPM repository is published.
COPR builds, publishing on pushes to `main`, and installer repository setup
are tracked in [#4](https://github.com/itsKontra/ryoku-fedora/issues/4).

`stage-package.sh` runs the existing Arch recipes' build/package functions in a
staging directory. It does not run package hooks. This keeps the CLI, providers,
QML modules, app helpers, translations and user units on the same payload map.
Fedora uses `/usr/lib64/qt6/qml`; Arch boot hooks are excluded. `ryostore` and
`ryovm` belong to the desktop payload, as on Arch. The empty `ryotunes` spec was
removed: that external application has no source in this checkout and is not
part of this RPM release. Install it separately from its upstream distribution.

## Build and source rebuild

On mutable Fedora, install `rpm-build rpm-sign createrepo_c golang gcc gcc-c++
cmake ninja-build pkgconf-pkg-config wayland-devel wayland-protocols-devel
ffmpeg-free-devel qt6-qtbase-devel qt6-qtdeclarative-devel
qt6-qtmultimedia-devel qt6-qtshadertools-devel qt6-qtsvg-devel
qt6-qt5compat-devel qt6-qtwayland-devel python3 gnupg2 git`.

Use a full-history checkout. Source preparation vendors dependencies pinned by
Go module sums into Source0. The SRPM contains that complete payload; `%build`
does not fetch source or rely on a checkout path. Rebuild it with
`mock -r fedora-44-x86_64 --rebuild <package.src.rpm>` in a clean root.

```sh
RYOKU_RPM_SIGNING_KEY=<fingerprint> RYOKU_RPM_OUT=/tmp/ryoku-release \
  release/rpm/build-rpm-repo.sh
```

Signed builds require a clean checkout, sign and verify every RPM, and sign
repository metadata. `RYOKU_RPM_UNSIGNED=1` explicitly enables local test builds;
never publish that output. The version `0.<git-commit-count>` increases along a
fast-forward release history. Release branches must share that history; do not
publish divergent branches at the same count. The output directory must be new,
so previous rollback packages cannot be overwritten accidentally. Archive times,
build host and build timestamps are fixed from the source commit. Use the same
Fedora toolchain/build root when comparing rebuilds.

## Repository contract

The repository ID is **ryoku**, regardless of the hosting provider. A bare COPR
ID is not that contract. Host signed output at:

```
<base>/channels/stable/<fedora-version>/x86_64/
<base>/channels/testing/<fedora-version>/x86_64/
<base>/releases/<tag>/<fedora-version>/x86_64/
```

Install `ryoku.repo.in` as `/etc/yum.repos.d/ryoku.repo`, replacing `@BASE_URL@`
with that base and `@KEY_URL@` with the publisher's verified public key URL.
Write the same base, alone on one line, to `/etc/dnf/vars/ryoku_baseurl`.
Keep `gpgcheck=1` and `repo_gpgcheck=1`. Each directory carries `release.json`
and signed `repodata/repomd.xml`; retain frozen release directories for rollback.
Publish only after the matching Fedora build/install gate passes. Upload a
frozen directory first, then atomically move the channel pointer onto it.

Install `ryoku-desktop` and the selected `ryoku-desktop-<provider>` together.
`ryoku track` changes only the `[ryoku]` baseurl. Updates refresh that repository
and run `distro-sync` with `--repo=ryoku`, restricting candidates to it; missing
new dependencies must be installed through the host's normal package transaction
before retrying. No unrestricted system downgrade is substituted.

DNF option semantics are documented in the
[DNF5 manual](https://dnf5.readthedocs.io/en/latest/dnf5.8.html) and
[distro-sync reference](https://dnf5.readthedocs.io/en/latest/commands/distro-sync.8.html).

## Source installs

The bootstrap defaults to `itsKontra/ryoku-fedora`, branch `main`.
Override both explicitly for another publisher:

```sh
RYOKU_SHELL_REPO=https://github.com/OWNER/ryoku-arch.git \
RYOKU_SHELL_REF=BRANCH bash ryoku-shell-installer/install.sh
```

`--repo` and `--ref` also work; the bootstrap applies them before fetching its
binary and checksum. Deployment records the checkout and update branch.
Recovery reuses that origin and branch and repairs Fedora dependencies before
resetting configuration. Immutable/OSTree installations are unsupported.

## Required VM evidence

For each supported Fedora version and both compositor choices, start from a
stock GNOME/KDE image without either compositor or any source deployment. Keep
SELinux enforcing. Record the image digest, `cat /etc/os-release`, `dnf --version`,
`dnf repolist`, `rpm -qa`, source commit, install log and first-update commit.

1. Install, reboot, and enter the selected session. Confirm the renderer,
   portals, polkit authentication, desktop QML, audio, lock/unlock, and screen
   sharing. Save `journalctl --user -b`, system SDDM logs and `ausearch -m AVC -ts boot`.
2. Test a password login, autologin, a locked keyring and an existing Chromium
   profile. The installer must not change the selected password-store backend.
3. Publish N and N+1 signed test channels, plus a competing repository offering
   a higher Ryoku version. Update and roll back; verify the selected channel
   supplies the packages and signature checking remains enabled.
4. Remove a required dependency and recover. Repeat offline: recovery must
   fail before deleting configuration. Check its origin and branch remain the
   installer's selections.
5. Place an unrelated `ryo-example` binary and custom user service before
   installation. Uninstall, verify both survive, original overwritten artifacts
   return, and recorded installed services stop.
6. On NVIDIA hardware, change modeset/driver configuration, check the dracut
   result and reboot. A container cannot validate that boot path.

Do not mark FED-56 or the SELinux finding complete without attaching those logs.
