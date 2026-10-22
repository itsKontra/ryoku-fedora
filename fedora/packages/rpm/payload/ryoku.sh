# shellcheck shell=bash
startdir=${startdir:?stage-package.sh must set startdir}
srcdir=${srcdir:?stage-package.sh must set srcdir}
pkgdir=${pkgdir:?stage-package.sh must set pkgdir}
pkgver=${RYOKU_PKGVER:?stage-package.sh must set RYOKU_PKGVER}
_repo="$startdir/../../../.."
_payload="$_repo/release/packages/ryoku/payload.sh"

build() {
  RYOKU_SRC="$_repo" RYOKU_BUILD_DIR="$srcdir" RYOKU_PKGVER="$pkgver" \
    "$_payload" build
}

package() {
  RYOKU_SRC="$_repo" RYOKU_BUILD_DIR="$srcdir" RYOKU_PKGVER="$pkgver" \
    "$_payload" install "$pkgdir" systemd
  install -Dm644 "$_repo/fedora/packages/translations.tsv" \
    "$pkgdir/usr/share/ryoku/packages/fedora.tsv"
  # a snapper pair around every dnf transaction outside `ryoku update`, through
  # the libdnf5 actions plugin.
  install -Dm755 "$_repo/fedora/system/snapshots/ryoku-dnf-snapshot" \
    "$pkgdir/usr/libexec/ryoku/ryoku-dnf-snapshot"
  install -Dm644 "$_repo/fedora/system/snapshots/ryoku-snapper.actions" \
    "$pkgdir/etc/dnf/libdnf5-plugins/actions.d/ryoku-snapper.actions"
  # ryoku-host kmod asks the driver script's --query whether the GPU gets the
  # NVIDIA driver, so the GSP check stays defined once.
  install -Dm755 "$_repo/system/hardware/drivers/nvidia.sh" \
    "$pkgdir/usr/libexec/ryoku/nvidia.sh"
}
