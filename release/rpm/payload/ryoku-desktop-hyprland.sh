# Maintainer: Ryoku <releases@ryoku.dev>
#
# ryoku-desktop-hyprland: the Hyprland compositor variant of the Ryoku desktop.
# Carries Hyprland, its plugins, its portal and satellites, the Hyprland config
# tree, and the ryoku-wm-hyprland provider binary (ryoku/wm/hyprland). It provides
# the ryoku-desktop-compositor virtual, so an installed ryoku-desktop pulls it on
# the next -Syu with no user action and `ryoku rollback` stays pure metadata.
# Its sibling may be installed beside it: keeping both makes a switch back
# instant, and `ryoku wm use --remove-previous` is the only thing that frees disk.
#
# built from in-repo sources, no tarball fetch. makepkg runs from a full
# checkout, so the repo root is three levels up.
pkgname=ryoku-desktop-hyprland
pkgver=${RYOKU_PKGVER:-0.1.0}
pkgrel=2
pkgdesc="Ryoku desktop: the Hyprland compositor, plugins, portal, and the ryoku-wm-hyprland provider"
# package() builds the Go provider, so the payload is arch-specific.
arch=('x86_64')
url="https://ryoku.dev"
license=('GPL-3.0-or-later')
makedepends=('go')
depends=(
  "ryoku-desktop=$pkgver"
  # compositor + Wayland session
  'hyprland'
  # Hyprland compositor plugins the Hub can toggle (Settings > Plugins), pinned to
  # this build so a plugin .so never runs against another release's headers. Off by
  # default; the generated settings.lua loads one only once the user enables it.
  "hypr-dynamic-cursors=$pkgver"
  "ryoku-hypr-plugins=$pkgver"
  "hyprglass=$pkgver"
  "imgborders=$pkgver"
  "ryoku-keysounds=$pkgver"
  'hyprpolkitagent'
  'xdg-desktop-portal-hyprland'
  # XDPH names /usr/bin/hyprland-preview-share-picker for the screen-share source
  # chooser, so a hard depend keeps the picker atomic with the portal.
  'hyprland-preview-share-picker'
  'hypridle'
  'hyprpicker'
)
provides=('ryoku-desktop-compositor')
# deliberately not exclusive: both variants may be installed, so a switch is a
# config change with no download and a switch back is instant.
source=()

_repo="$startdir/../../.."

package() {
  local cfg="$pkgdir/usr/share/ryoku/config"

  # Hyprland config tree: includes scripts/ (shell calls ~/.config/hypr/scripts/*
  # by absolute path) and the hardware-managed gpu/keyboard/monitors drop-ins.
  install -d "$cfg/hypr"
  cp -a "$_repo/ryoku/hyprland/." "$cfg/hypr/"

  # Fedora's XDPH package ships its supported picker at this name. The custom
  # preview picker is Arch-only and must not leave the RPM transaction with an
  # unavailable dependency or the installed portal pointing at a missing tool.
  sed -i 's/custom_picker_binary = hyprland-preview-share-picker/custom_picker_binary = hyprland-share-picker/' \
    "$cfg/hypr/xdph.conf"

  # xdg-desktop-portal: route ScreenCast/Screenshot to Hyprland for screen sharing.
  install -Dm644 "$_repo/ryoku/hyprland/hyprland-portals.conf" \
    "$cfg/xdg-desktop-portal/hyprland-portals.conf"

  # cp -a kept source modes; keep hypr/scripts executable, the rest world-readable.
  chmod -R u=rwX,go=rX "$pkgdir/usr/share/ryoku"

  # the Hyprland leaf scripts the keybinds, autostart and the shell call by bare
  # name. Same ryoku-* glob the dev deploy uses, so a new script lands in both.
  local s
  for s in "$_repo"/ryoku/hyprland/scripts/ryoku-*; do
    [[ -f $s ]] || continue
    install -Dm755 "$s" "$pkgdir/usr/bin/${s##*/}"
  done

  # ryoku-wm-hyprland: the Hyprland half of the window-manager seam. Built from the
  # ryoku/wm module (its root is one dir up from the hyprland package).
  ( cd "$_repo/ryoku/wm/hyprland" && go build -o ryoku-wm-hyprland . )
  install -Dm755 "$_repo/ryoku/wm/hyprland/ryoku-wm-hyprland" \
    "$pkgdir/usr/bin/ryoku-wm-hyprland"

  # Ryoku.Wm.Hyprland: the provider's QML bridges (global shortcuts, focus grab)
  # the shell imports instead of Quickshell.Hyprland. Pure QML, like Ryoku.Ui.
  install -d "$pkgdir/usr/lib/qt6/qml/Ryoku/Wm/Hyprland"
  cp -a "$_repo/ryoku/wm/hyprland/qml/." "$pkgdir/usr/lib/qt6/qml/Ryoku/Wm/Hyprland/"
}
