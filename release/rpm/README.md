# Fedora RPM delivery

The port is experimental. Container builds verify package construction and
installation; graphical sessions and SELinux require the VM checks below.
Nothing in this directory implies that a public RPM repository is published.
COPR builds, publishing on pushes to `main-fedora`, and installer repository setup
are implemented by `publish-copr.yml`; enabling the infrastructure and recording
end-to-end evidence remain tracked in [#4](https://github.com/itsKontra/ryoku-fedora/issues/4).

The initial target is mutable Fedora 44 x86_64. `itskontra/ryoku` must be
created with the `fedora-44-x86_64` chroot. The existing third-party dependency
COPRs are declared once in `dependency-coprs`, consumed by the installer and
container tests. `itskontra/ryotunes` remains an optional external project;
its `ryoku-ui` ownership must be reconciled before enabling it with the desktop.

The first publishing implementation retains `ryoku-extras` as the package
owner of pinned tools and assets. Packaged installs therefore use DNF for
matugen, gpk, prowl-agent, fonts and cursors, without running their download
helper. Splitting the executable payload into source-built packages in
`itskontra/ryoku-deps` remains a separate follow-up; no installer enables that
project before it has working packages. Fedora 44's matugen 3.1 is not a
substitute for the currently pinned 4.2 without compatibility validation.

**Publisher setup**

Create a GitHub environment named `fedora-publish`. Add these repository
variables (the signature-only install jobs must be able to read them too):

| Variable | Value |
|---|---|
| `RYOKU_RPM_BASE_URL` | Required HTTPS root of the Fedora release tree. No default host is assumed. |
| `RYOKU_COPR_FINGERPRINT` | Full fingerprint of the `itskontra/ryoku` COPR key. |
| `RYOKU_RPM_SIGNING_KEY` | Full fingerprint of the release metadata signing key. |
| `RYOKU_RPM_PUBLISH_HOST` | SSH destination, such as `publisher@packages.example.org`. |
| `RYOKU_RPM_PUBLISH_ROOT` | Absolute filesystem directory served at the base URL. |

Add environment secrets `COPR_CONFIG` (the COPR API configuration, with builder
access), `RYOKU_RPM_METADATA_PRIVATE_KEY` (armored signing key usable by batch
GPG), `RYOKU_RPM_SSH_KEY`, and `RYOKU_RPM_KNOWN_HOSTS` (verified SSH host keys).
The release host needs Python 3, rsync, and a filesystem supporting symlinks
and atomic rename. Its HTTPS server must follow the channel symlinks. Keep
the `.incoming` directory and `.publish.lock` inaccessible through HTTP.
The SSH account only needs write access to the dedicated Fedora release root.

Main-fedora pushes and manual runs on main-fedora publish testing snapshots. Version tags
reachable from main-fedora build and test a stable candidate. Tags must match the
updater's release syntax, for example `v1.0.0` or `v1.0.0-beta.1`. Each run
uses its run number as the RPM Release revision. All components of a candidate
share that revision. Stable promotion uses that run's tested COPR artifacts,
not a second rebuild after the gate.

The reusable Fedora workflow prepares SRPMs once, rebuilds every one in clean
Mock roots, then installs their output on both providers with DNF5 and DNF4.
Only a passing run can submit those same SRPMs to COPR. The publisher records
build IDs, waits for every selected chroot, downloads those exact RPMs, and
checks their signatures against the pinned COPR fingerprint. A second install
gate verifies the unchanged COPR RPMs and signed metadata before publishing.
PR jobs have no publishing credentials. Failed builds never move the client
channel, even when individual COPR builds have already become visible.

The release host first receives a candidate under `.incoming`. `promote.py`
locks publication, installs a frozen `releases/<tag>/44/x86_64` tree, records
stable releases in `releases/index.json`, and atomically replaces the channel
symlink. Earlier or repeated run numbers cannot move a channel backward.
Frozen releases are never overwritten. If transfer fails, inspect and remove
only that incomplete `.incoming/<tag>` before retrying; a completed release
requires a new run/revision. Back up frozen releases independently of COPR's
retention policy.
Package locations inside repository metadata point to the frozen release,
relative to the shared repository layout. A client using cached metadata can
therefore finish its downloads even if the channel advances during its transaction.

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

`RYOKU_SRPM_OUT=/tmp/ryoku-srpms release/rpm/prepare-srpms.sh` produces only
self-contained source RPMs and their checksum manifest. It requires a clean
checkout unless `RYOKU_RPM_LOCAL=1` explicitly selects development input.
`release/rpm/rebuild-srpms.sh /tmp/ryoku-srpms /tmp/ryoku-rebuilt` rebuilds the
entire set with Mock. Build-root network access is not enabled for compilation.

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

The raw desktop COPR repository is a build output, not a supported client
channel. Do not enable it alongside `[ryoku]`: its individual builds bypass
whole-desktop install gating. The configured `[ryoku]` consumes the promoted
COPR-signed RPMs. Dependency COPRs retain their normal DNF IDs.

DNF option semantics are documented in the
[DNF5 manual](https://dnf5.readthedocs.io/en/latest/dnf5.8.html) and
[distro-sync reference](https://dnf5.readthedocs.io/en/latest/commands/distro-sync.8.html).

## Installer modes

For a packaged Fedora install, export `RYOKU_RPM_BASE_URL`,
`RYOKU_COPR_FINGERPRINT`, and `RYOKU_RPM_SIGNING_KEY` with the published values,
then run the shell installer with `--install-mode=packages`. It validates
downloaded public keys before writing `[ryoku]`, installs the desktop and
selected provider with DNF, and materializes their config. The initial install
selects testing; `ryoku track stable` switches once stable has been published.
Missing configuration or unavailable packages fail explicitly, with no automatic
fallback to compilation.

`--install-mode=auto` is the default: Fedora uses packages unless a local payload
or a custom repository/ref was selected. `--install-mode=source` always keeps Fedora's
checkout build. Arch and Debian retain their existing installation behavior.
Source builds continue to use locally compiled Ryoku binaries, app helpers,
and QML modules. The bootstrap executable remains a checksummed download.

Switching a source install to packages restores/removes artifacts tracked by
the source installer before clearing source tracking. Modified tracked files,
or a source checkout marker without an ownership receipt, block conversion
with an actionable error instead of deleting untracked files. Resume state
is scoped to the mode and provider. Recovery deployments without receipts
still need a separate migration audit; the broader doctor cleanup is not
changed by this implementation.

## Source installs

The bootstrap defaults to `itsKontra/ryoku-fedora`, branch `main-fedora`.
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
