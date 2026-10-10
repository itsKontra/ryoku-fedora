# Maintainer: Ryoku <releases@ryoku.dev>
#
# Ryoku control CLI (Go): the single front door to update, rollback, snapshot,
# and the config materialize step.
#
# RPM metadata and dependencies live in release/rpm/ryoku.spec. This sourced
# recipe only builds from ryoku/cli and stages the payload for stage-package.sh.
# The keyring subcommand's godbus is vendored, so the signed-repo CI builds
# offline.
startdir=${startdir:?stage-package.sh must set startdir}
srcdir=${srcdir:?stage-package.sh must set srcdir}
pkgdir=${pkgdir:?stage-package.sh must set pkgdir}
_repo="$startdir/../../.."

build() {
  cd "$_repo/ryoku/cli" || return
  CGO_ENABLED=0 go build -trimpath -mod=vendor -o "$srcdir/ryoku" .
}

package() {
  install -Dm755 "$srcdir/ryoku" "$pkgdir/usr/bin/ryoku"
  # the boot guard: a system unit `ryoku boot-guard` runs from early in every
  # boot (enabled by the doctor on every update, so existing boxes get it),
  # and the tmpfiles entries for its marker and the sessions' boot-ok files.
  install -Dm644 "$_repo/ryoku/cli/systemd/ryoku-boot-guard.service" \
    "$pkgdir/usr/lib/systemd/system/ryoku-boot-guard.service"
  install -Dm644 "$_repo/ryoku/cli/systemd/ryoku.tmpfiles.conf" \
    "$pkgdir/usr/lib/tmpfiles.d/ryoku.conf"
  # the canonical snapper root config the doctor embeds, for the Fedora
  # installer to seed before the first update runs.
  install -Dm644 "$_repo/ryoku/cli/internal/doctor/snapper-root.conf" \
    "$pkgdir/usr/share/ryoku/snapper/root.conf"
  # the dnf hook (system/snapshots): a snapper pair around every dnf
  # transaction outside `ryoku update`, through the libdnf5 actions plugin.
  install -Dm755 "$_repo/system/snapshots/ryoku-dnf-snapshot" \
    "$pkgdir/usr/libexec/ryoku/ryoku-dnf-snapshot"
  install -Dm644 "$_repo/system/snapshots/ryoku-snapper.actions" \
    "$pkgdir/etc/dnf/libdnf5-plugins/actions.d/ryoku-snapper.actions"
}
