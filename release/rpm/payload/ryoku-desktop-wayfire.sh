# Maintainer: Ryoku <releases@ryoku.dev>
#
# ryoku-desktop-wayfire: the Wayfire compositor variant of the Ryoku desktop.
# Carries the Wayfire config baseline and the ryoku-wm-wayfire provider binary
# (ryoku/wm/wayfire). It provides the ryoku-desktop-compositor virtual, so an
# installed ryoku-desktop is satisfied by it, and may be installed beside the
# other variants: a switch is then a config change, not a package swap.
#
# RPM metadata and dependencies live in ryoku-desktop-wayfire.spec. This sourced
# recipe only stages the payload assembled by stage-package.sh.
#
# Every plugin the baseline's core/plugins loads (ipc and ipc-rules included)
# ships in Fedora's own wayfire package, so nothing is built here beyond the
# provider. Wayfire manages Xwayland but does not pull the server, hence the
# spec's xorg-x11-server-Xwayland. Screen sharing rides xdg-desktop-portal-gnome
# and night light rides wlsunset over wlr-gamma-control.
#
# No systemd drop-in, unlike niri: wayfire ships no user unit, so there is no
# unit to order the config bootstrap against. ryoku-bootstrap runs globally for
# this case, and wayfire's own inotify reload picks up a tree materialized
# moments after login.
startdir=${startdir:?stage-package.sh must set startdir}
pkgdir=${pkgdir:?stage-package.sh must set pkgdir}
_repo="$startdir/../../.."

package() {
  local cfg="$pkgdir/usr/share/ryoku/config"

  # wayfire config tree: the shipped baseline, the first layer the provider's
  # apply composes (the live wayfire.ini is apply's render, not this file; see
  # caps.GeneratedFiles).
  install -d "$cfg/wayfire"
  cp -a "$_repo/ryoku/wayfire/." "$cfg/wayfire/"

  # xdg-desktop-portal: wayfire implements no portal backend of its own, so
  # screen sharing rides the GNOME backend but the GNOME FileChooser hangs off a
  # GNOME session. This routing pins FileChooser to gtk so file pickers open;
  # doctor's portal reconciler reads it by the running desktop's name.
  install -Dm644 "$_repo/ryoku/wayfire/wayfire-portals.conf" \
    "$cfg/xdg-desktop-portal/wayfire-portals.conf"

  # cp -a kept source modes; the tree is all data, so world-readable is right.
  chmod -R u=rwX,go=rX "$pkgdir/usr/share/ryoku"

  # ryoku-wm-wayfire: the wayfire half of the window-manager seam. Built from
  # the ryoku/wm module (its root is one dir up from the wayfire package).
  ( cd "$_repo/ryoku/wm/wayfire" && go build -o ryoku-wm-wayfire . )
  install -Dm755 "$_repo/ryoku/wm/wayfire/ryoku-wm-wayfire" \
    "$pkgdir/usr/bin/ryoku-wm-wayfire"
}
