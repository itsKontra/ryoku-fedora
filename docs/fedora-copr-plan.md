Implementation plan for [issue #4](https://github.com/itsKontra/ryoku-fedora/issues/4), inspected 2026-09-20. This is a proposal, not a record of completed publishing or install validation.

Use COPR to build Fedora RPMs, use DNF to install released software, and retain checkout builds for Ryoku development. Keep the existing signed `[ryoku]` channel and frozen-release contract by promoting tested COPR output into a release repository.

**Current state and gaps**

- `release/rpm/` has eleven specs: `ryoku`, `ryoku-shell`, `ryoku-hub`, `ryoku-rashin`, `ryogami`, `ryoku-blobs`, `ryoku-desktop`, both desktop provider variants, `ryoku-extras`, and `sddm-theme-ryoku`. `ryostore` and `ryovm` are desktop payload, not separate RPMs.
- `build-rpm-repo.sh` vendors Go dependencies and bundles extras into Source0, then runs `rpmbuild -ba` for every spec. It needs a source-only entry point for COPR submission.
- `.github/workflows/fedora-rpm.yml` currently exists as an untracked working-tree file. It tests Fedora 44, both providers, and DNF/DNF4, on pull requests or manual dispatch. It has no push trigger or COPR publishing job. Preserve this existing work when implementing the plan.
- `installation/tests/fedora-rpm.sh` re-signs all binary RPMs with a disposable key, including prebuilt input. It rebuilds only the hub SRPM in the already populated container. Neither verifies all clean-root builds nor preserves publisher signatures for a release gate.
- Fedora has `fromSource: true` in `ryoku-shell-installer/distro.go`. `stepPackages` installs dependencies and compilers; `stepBuild` runs `ryoku/shell/deploy.sh`.
- Installer and test independently list five dependency COPRs: `sdegler/hyprland`, `errornointernet/quickshell`, `atim/starship`, `atim/lazygit`, and `lihaohong/yazi`.
- `ryoku-install-extra` downloads upstream binaries for `matugen`, `gpk`, and `prowl-agent`, plus Bibata, Space Grotesk, Material Symbols, and JetBrains Mono Nerd Font assets. Those binaries are not compiled from this checkout. `ryoku-extras.spec` currently bundles all of them into an RPM.
- `ryoku/cli/internal/sys/rpm.go` expects `/etc/yum.repos.d/ryoku.repo`, repository ID `ryoku`, and `/etc/dnf/vars/ryoku_baseurl`. A normal COPR enable operation does not satisfy that contract.
- `reconcileDevResidue` in `ryoku/cli/internal/doctor/doctor.go` still invokes `pacman -Qoq` to identify local binary shadows. RPM migration must fix this, including packaged names such as `ryogami` that do not start with `ryoku`.

**COPR projects and initial support**

| Project | Action | Contents and purpose |
|---|---|---|
| `itskontra/ryoku` | Create | The eleven existing desktop RPMs, built together from one repository commit. This is build output; supported client channels receive complete, tested sets through promotion. |
| `itskontra/ryoku-deps` | Create | Individually versioned `matugen`, `gpk`, and `prowl-agent` packages, replacing direct executable downloads. Add other upstream dependencies only when Fedora lacks a compatible package or Ryoku needs a maintained pin. |
| `itskontra/ryotunes` | Reuse | Already exists for Fedora 44 x86_64. Successful builds observed: `ryotunes` 1.0.8, build 11007017, and `ryoku-ui` 0.1, build 11007006. Keep the external application's release lifecycle separate. |

The public [project API](https://copr.fedorainfracloud.org/api_3/project/list?ownername=itskontra) returned only `ryotunes` at inspection time; the [build API](https://copr.fedorainfracloud.org/api_3/build/list?ownername=itskontra&projectname=ryotunes&limit=5) supplied the successful build states. These facts do not establish a working full desktop install.

Start with mutable **Fedora 44 x86_64**, matching the existing test matrix and ryotunes project. Confirm a clean buildroot supplies the specs' `golang >= 1.26.4` and required Qt versions before advertising support. Other Fedora releases, aarch64, and immutable systems are outside the initial supported matrix; add a release only with matching dependency availability and install/update evidence.

One project can contain many packages; do not create a project per desktop binary. Keep working third-party dependency projects initially, but query availability and compatible versions in a clean Fedora 44 container. Prefer official Fedora packages when suitable. Enable optional application repositories only when needed. In particular, inspect `awww`, Quickshell, the chosen provider's helpers, and optional starship/lazygit/yazi sources rather than assuming today's rename table proves availability.

For `ryoku-deps`, build pinned upstream source releases with their required vendored dependencies. Retain `ryoku-extras` for assets and dependencies, removing its ownership of the three executable paths as the new packages take over. This avoids two packages owning `/usr/bin/matugen`, `/usr/bin/gpk`, or `/usr/bin/prowl-agent`. Preserve licenses and install the asset licenses alongside the files. Add dependency specs without making the desktop build loop accidentally rebuild every external project on every push.

Before enabling ryotunes with the desktop, inspect both RPM file lists: its existing `ryoku-ui` package may overlap the `Ryoku.Ui` module currently shipped by `ryoku-desktop`. Establish one RPM owner and compatible version requirements. If splitting `ryoku-ui` out, build it from this monorepo and make both consumers depend on it; coordinate removal of the older duplicate recipe. An optional application must not replace a developer's local QML module.

**Repository and channel design**

Retain the layout documented in `release/rpm/README.md`:

```text
COPR build output -> verify signatures -> test exact RPM set
                 -> immutable release directory -> channel promotion

<base>/channels/testing/44/x86_64/
<base>/channels/stable/44/x86_64/
<base>/releases/<tag>/44/x86_64/
```

Use a Fedora-specific hosting prefix and signing configuration, following the existing Arch publisher's build/test/promote structure. Preserve COPR RPM signatures; generate repository metadata for the selected set and sign `repomd.xml` with the release metadata key. Configure both trusted keys as necessary and keep `gpgcheck=1` and `repo_gpgcheck=1` for `[ryoku]`. Publish `release.json` and the release ledger used by track/status/rollback. Do not promote a partially built set: the desktop spec pins component versions exactly.

COPR's normal repository URL and ID alone cannot provide the existing channel URLs, release metadata, and durable rollback history. Its documented pruning policy also makes default build retention unsuitable for a frozen release archive. See [COPR user documentation](https://docs.copr.fedorainfracloud.org/user_documentation.html#how-long-do-you-keep-the-builds).

The installer enables required dependency COPRs through DNF. For desktop packages, configure canonical `[ryoku]` backed by promoted COPR artifacts. If desktop COPR enablement is used to bootstrap its configuration/key, disable its generated raw repository afterward; otherwise ordinary DNF operations could install an unpromoted build. Document this distinction explicitly. Pure COPR-only hosting would instead require a broader redesign of channel and rollback handling, not just renaming a repository ID.

Publish main-fedora pushes to **testing**; promote a tested tagged commit to **stable**, consistent with the existing release convention. Issue #4 requires publication on main-fedora pushes but does not require treating every push as a stable release. Never label a tag's newly rebuilt, untested artifacts as the previously tested release.

**GitHub workflow**

Implement `.github/workflows/publish-copr.yml`, with `push` to `main-fedora` and manual dispatch restricted to the trusted main-fedora ref. Add reusable invocation support to `fedora-rpm.yml`; PR validation stays credential-free. COPR supports uploaded SRPMs and monitored submissions through [copr-cli](https://developer.fedoraproject.org/deployment/copr/copr-cli.html).

1. Prepare SRPMs once from the exact full-history commit. Extract source preparation from `build-rpm-repo.sh` and use `rpmbuild -bs`; retain the existing local binary/repository builder as a consumer. Include vendored Go modules, local module replacements, packaging recipes, assets, and `.rpm-release`. Avoid network fetches in `%build`. Record commit, version, source checksums, and package list.
2. Rebuild all desktop SRPMs in clean Fedora 44 mock roots, then run the install/channel checks for both providers and DNF versions. Audit BuildRequires against clean roots instead of relying on the container's broad tool list. This gate must succeed before COPR submission.
3. Submit the exact validated SRPM artifacts to `itskontra/ryoku`. Build required dependency projects first when their pinned versions change. Track every build ID/chroot, wait for terminal success, and fail the job on failure, cancellation, or timeout. Report links and logs in the job summary. A successful submission is not a successful build.
4. Download the exact RPM outputs belonging to those IDs, verify against the recorded COPR public-key fingerprint, and run an install/update gate without deleting or replacing signatures. Do not read an arbitrary latest repository state while another publish may be running.
5. Assemble the immutable release candidate, sign metadata, test that repository, and promote its testing channel only after every check passes. Serialize promotion and prevent an older run from replacing a newer channel. Stable promotion reuses the tested artifacts. Rebuilds at unchanged source need a coherent RPM Release revision across version-pinned components if their bytes change.
6. Retain SRPMs, package checksums, build IDs, logs, release metadata, and install/update evidence. A failed run must leave the prior client channel intact, even if COPR already exposes individual successful builds.

Maintainer setup: create the two projects and Fedora 44 x86_64 chroots; configure necessary build repositories; record public-key fingerprints; provision the release host and metadata key; and grant CI builder access. Store COPR API configuration in a GitHub environment secret such as `COPR_CONFIG`, write it to a temporary mode-0600 file, and remove it after use. Keep owner/project names in non-secret variables. Hosting credentials and the metadata signing key belong only in the promotion job. Do not expose these credentials to PR jobs or use `pull_request_target` to execute PR code.

**Where to use DNF, and where to retain local builds**

| Component/path | Normal Fedora install | Checkout development |
|---|---|---|
| `ryoku`, shell, hub, rashin, provider binaries | `dnf install ryoku-desktop ryoku-desktop-<provider>` | Keep local Go builds and `~/.local/bin` deployment. |
| `ryogami`, `ryogami-live`, app backends/helpers including ryostore/ryovm | Install their RPM payloads | Keep checkout Go/C builds and scripts so uncommitted edits are exercised. |
| `Ryoku.Blobs`, `Ryoku.Ui`, shell/app QML | RPM-owned modules/data and `ryoku materialize` | Keep local modules/QML and Qt-dependent rebuilds. |
| matugen, gpk, prowl-agent | DNF packages from Fedora or `itskontra/ryoku-deps` | DNF packages too; the checkout does not build them. |
| Bibata, Space Grotesk, Material Symbols | RPM-owned assets via `ryoku-extras` or suitable Fedora packages | DNF assets too, unless deliberately developing those assets. |
| ryotunes | Optional `dnf install ryotunes` from existing project, after UI ownership fix | Same packaged external app; this checkout has no ryotunes source. |
| Quickshell, compositors, portals, other system dependencies | DNF from verified repositories | DNF too, unless explicitly developing that external project. |
| Go/C/C++/Qt build tools | Needed in builders; no desktop compile step on target | DNF installs the toolchain used by deploy/dev-run. |
| Installer bootstrap executable | Keep checksum-verified download until repository setup exists | Keep local installer build when testing installer changes. |

Installing a locally produced RPM using `dnf install ./package.rpm` is also valid for package testing. It preserves that local build while letting DNF resolve dependencies; it must not be confused with installing a published package by name. Keep clean-build test inputs local to the run so tests cannot silently exercise last week's COPR binaries.

Implement an explicit installation mode independent of distro. Simply changing Fedora's `fromSource` boolean would enter Arch-oriented package and step paths, including keyring/driver handling. Update `newEngine`, `stepPackages`, `stepConfigs`, provider checks, verification, and uninstall ownership together. Preserve source selection for developer payload/ref use and show the chosen mode clearly. Do not silently fall back to compiling if package delivery fails.

Use one repository/dependency declaration for installer and Fedora tests. Preserve DNF4/DNF5 plugin detection already present in `stepTools`; the current test hardcodes `dnf5-plugins` and runs COPR setup through `dnf`, even in its DNF4 matrix. Preflight selected chroot, metadata, signatures, packages, and provider compatibility before conflict removal; errors should identify the repository/package and retry action. Required matugen delivery must not be reported as an optional-download warning.

Disable direct extras downloads for package-owned payloads. For migration, use RPM file ownership plus existing installer/download receipts to retire only known Ryoku-managed local shadows. Include local QML imports, desktop entries, user service ExecStart overrides, and source tracking markers. Preserve unrelated user scripts and preserve all local artifacts when source mode remains selected. Keep `bin/ryoku-recovery`'s deliberate checkout rebuild behavior, but verify the return to packaged mode actually removes its managed shadows. `ryoku deploy`, dev-run, and source tracking must continue using local builds.

**Implementation order and acceptance**

1. Confirm project/chroot/toolchain choices and the release host; resolve ryotunes UI ownership and dependency package ownership.
2. Extract source preparation, add dependency recipes, and prove every SRPM rebuilds in a clean root.
3. Implement reusable Fedora checks, COPR submit/wait, signature-preserving output tests, and atomic channel promotion.
4. Add the installer package mode and shared repository setup; implement mode-aware cleanup, recovery, and updater integration.
5. Validate fresh installation without a compiler, N-to-N+1 update, tag rollback, and a competing repository with a higher version. Exercise both providers and DNF versions. Verify package signatures, selected repository, service executable paths, QML imports, and preservation of user overlays.
6. Test unavailable repos/packages, rejected signatures, failed/cancelled builds, offline recovery, and migration from the existing source install. Separately prove local edits still run after a development deploy.
7. Run Fedora VM checks with SELinux enforcing: login, lock/unlock, portals/screen sharing, audio, SDDM, and the documented hardware cases. Record exact image, commit, RPM list, repository configuration, and logs. Container success alone does not complete graphical/SELinux acceptance.

Update `release/rpm/README.md`, installer documentation/changelog, and the Fedora checklist with the final project names, key fingerprints, setup steps, support matrix, and evidence. This planning pass did not run Fedora builds or VM tests and did not change COPR, GitHub secrets, or the live desktop.

**Implementation status (2026-09-20)**

Implemented source preparation, clean Mock rebuilds, reusable install gates,
COPR submission and result tracking, signature-preserving repository assembly,
and atomic channel promotion. `RYOKU_RPM_BASE_URL` is a required deployment
variable, as requested; there is no placeholder public repository. The setup
variables, secrets, host layout and project requirements are documented in
[the RPM guide](../release/rpm/README.md).

Normal Fedora installs now select DNF packages. Explicit source mode, local
payloads, custom repositories and non-main-fedora refs retain checkout builds.
Migration removes only unchanged artifacts covered by the source installation
receipt and refuses unknown shadows before removing conflicting packages.

Validation used Fedora 44 x86_64 containers. All 11 SRPMs built successfully in
clean Mock roots. Fresh niri/DNF5 and Hyprland/DNF4 installs passed with locally
rebuilt RPMs; Hyprland/DNF5 and niri/DNF4 passed while preserving candidate RPM
signatures. These checks include materialization and channel upgrade/downgrade
fixtures. A separate real RPM test verified distinct package/metadata keys,
rejected modified packages and wrong fingerprints, and installed through frozen
release URLs. The candidate was signed locally for testing, not built on COPR.

Remaining issue work: create/configure the external infrastructure and run the
real publisher; split pinned executable extras into source-built dependency
packages; reconcile optional ryotunes UI ownership; complete recovery-to-package
migration for installations without usable receipts; and record real release
upgrade/rollback plus graphical and SELinux VM evidence. Existing `ryoku-extras`
remains the DNF-owned delivery unit for pinned tools and assets in this first
implementation.
