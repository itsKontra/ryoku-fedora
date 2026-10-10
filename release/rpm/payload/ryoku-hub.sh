# Maintainer: Ryoku <releases@ryoku.dev>
#
# Ryoku Hub backend (Go). A standalone helper the Hub's Quickshell config shells
# out to for the keybind legend and the Hub's TOML config. It parses config
# files and prints results, so it has no runtime dependencies of its own.
#
# Built from the in-repo source at ryoku/hub/backend; no tarball is fetched.
# stage-package.sh exports GOFLAGS (-mod=vendor -trimpath) and CGO_ENABLED=0, so
# go build reads the committed vendor/ tree and the signed-repo CI builds with no
# network. RPM metadata lives in ryoku-hub.spec.
startdir=${startdir:?stage-package.sh must set startdir}
srcdir=${srcdir:?stage-package.sh must set srcdir}
pkgdir=${pkgdir:?stage-package.sh must set pkgdir}
_repo="$startdir/../../.."

build() {
  cd "$_repo/ryoku/hub/backend" || exit
  go build -o "$srcdir/ryoku-hub" .
}

package() {
  install -Dm755 "$srcdir/ryoku-hub" "$pkgdir/usr/bin/ryoku-hub"
}
