# Maintainer: Ryoku <releases@ryoku.dev>
#
# Ryogami: the Ryoku wallpaper daemon (Go). One process owns the wallpaper
# catalog (thumbnails, colours, favourites), applies through the in-shell
# surface topic, the matugen palette, and the depth surface, behind ryoku's
# line pub/sub on $XDG_RUNTIME_DIR/ryogami.sock. The vendored skwd-wall picker
# (wall-ui) is its front end.
#
# built from the in-repo Go module at ryoku/shell/ryogami/daemon, no tarball
# fetched. stage-package.sh sources this recipe from release/rpm/payload, so
# the repo root is three levels up; go builds into $srcdir so the source tree
# stays untouched.
#
# RPM metadata and Requires live in ryogami.spec. The daemon shells out to
# ImageMagick and ffmpeg (thumbnails, colour extraction, the livewall
# transcode), matugen (palette on apply) and quickshell (the wall-ui). Video
# wallpapers play through the bundled ryogami-live, built here from
# ryoku/shell/livewall. The wall-ui source browser fetches remote wallpapers
# (Wallhaven, MoeWalls, MotionBGs, GitHub, Steam) with curl, and its folder
# watcher execs inotifywait (inotify-tools); without it a wallpaper that lands
# while the picker is open never shows until the picker is reopened.
startdir=${startdir:?stage-package.sh must set startdir}
srcdir=${srcdir:?stage-package.sh must set srcdir}
pkgdir=${pkgdir:?stage-package.sh must set pkgdir}
_repo="$startdir/../../.."

build() {
  cd "$_repo/ryoku/shell/ryogami/daemon" || exit
  # the GOFLAGS stage-package.sh exports (-mod=vendor) keep the build off
  # proxy.golang.org, so it is reproducible and works in the offline publish
  # container; the module is stdlib-only, so there is no vendor/ tree to carry.
  go build -o "$srcdir/ryogami" .
  "$_repo/ryoku/shell/livewall/build.sh" "$srcdir/ryogami-live"
}

package() {
  install -Dm755 "$srcdir/ryogami" "$pkgdir/usr/bin/ryogami"
  install -Dm755 "$srcdir/ryogami-live" "$pkgdir/usr/bin/ryogami-live"
  # user unit: systemd finds it under /usr/lib/systemd/user without materialize,
  # exactly where ExecStart already points (/usr/bin/ryogami).
  install -Dm644 "$_repo/ryoku/shell/systemd/user/ryogami.service" \
    "$pkgdir/usr/lib/systemd/user/ryogami.service"
  # the wall-ui picker (vendored skwd-wall, MIT): the quickshell config the
  # daemon spawns; /usr/share/ryogami/shell.qml is the resolver's packaged
  # default, so no env override is needed on an installed box.
  install -d "$pkgdir/usr/share/ryogami"
  cp -a "$_repo/ryoku/shell/ryogami/wall-ui/." "$pkgdir/usr/share/ryogami/"
  install -Dm644 "$_repo/ryoku/shell/ryogami/wall-ui/data/ryogami-wall.desktop" \
    "$pkgdir/usr/share/applications/ryogami-wall.desktop"
  install -Dm644 "$_repo/ryoku/shell/ryogami/wall-ui/LICENSE" \
    "$pkgdir/usr/share/licenses/ryogami/LICENSE"
}
