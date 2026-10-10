#!/usr/bin/env bash
# Prevent the "feature wired but its tool is not shipped" class: every external
# program a Ryoku desktop feature depends on must reach every Fedora box through
# the RPM specs. This is a curated feature -> package map; add a row when a new
# feature starts shelling out to a new tool.
set -euo pipefail

ROOT=${RYOKU_PATH:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}
rpm_dir="$ROOT/release/rpm"

# hard_depend: $1 is a Requires of the ryoku-desktop umbrella, of a compositor
# variant it pulls through its ryoku-desktop-compositor virtual (a tool only that
# compositor's features shell out to, like hyprsunset, reaches every box that can
# run them that way), or of a first-party package the umbrella requires (ryogami
# carries inotify-tools). That is what lands on the ISO, on `ryoku update` and
# via the shell installer alike. This is the guard the ddcutil regression
# slipped past: ddcutil was in a package list but not a depend, so the pill
# DISPLAY faders were dead on every packaged box.
desktop_specs=("$rpm_dir/ryoku-desktop.spec" "$rpm_dir"/ryoku-desktop-*.spec)
requires() {
  grep -qE "^Requires:[[:space:]]+$1([[:space:]]|$)" "$2"
}
hard_depend() {
  local spec name desktop
  for spec in "${desktop_specs[@]}"; do
    [[ -f $spec ]] || continue
    requires "$1" "$spec" && return 0
  done
  for spec in "$rpm_dir"/*.spec; do
    requires "$1" "$spec" || continue
    name=$(awk '/^Name:/ { print $2; exit }' "$spec")
    for desktop in "${desktop_specs[@]}"; do
      requires "$name" "$desktop" && return 0
    done
  done
  return 1
}
# bundled: a first-party package the umbrella requires installs the tool's
# binary itself, like ryoku-extras shipping the pinned matugen over Fedora's.
bundled() {
  local spec name
  for spec in "$rpm_dir"/*.spec; do
    grep -qxF "%{_bindir}/$1" "$spec" || continue
    name=$(awk '/^Name:/ { print $2; exit }' "$spec")
    hard_depend "$name" && return 0
  done
  return 1
}
# shipped_app: the other delivery path. An application a user may delete is not a
# hard depend (dnf would put it back on the next upgrade); `ryoku doctor`
# delivers it once and then honours the removal. Membership is the manifest's own
# table, so a name cannot fall out of delivery and still pass this gate.
shipped_apps_go="$ROOT/ryoku/cli/internal/ryokumanifest/manifest.go"
shipped_app() {
  grep -qE "^[[:space:]]*\{\"$1\", " "$shipped_apps_go"
}
# deliberately neither a hard depend nor a provisioned app (documented exceptions):
#   chromium     -- the default browser is user-swappable; the ISO's packages.list
#                   ships it and the ryoku-app role resolver tolerates another
#                   browser being set.
#   voxtype-bin, -- no Fedora package yet (installation/fedora/README.md lists the
#   nvibrant-bin    porting gaps); the features stay gated off until one exists.
declare -A dependExempt=( [chromium]=1 [voxtype-bin]=1 [nvibrant-bin]=1 )

# feature -> package that provides it
declare -A need=(
  # the wallpaper daemon is ryogami, a first-party [ryoku] package and a hard
  # ryoku-desktop depend; live wallpapers ride ryoku-livewall, which ships
  # inside the ryoku-shell package itself (phonto/mpvpaper were dropped with
  # it, 7c20f7dd; awww was retired with 25a57f57).
  [wallpaper-daemon]=ryogami
  [palette]=matugen
  [clipboard-history]=cliphist
  [color-picker]=hyprpicker
  [music-visualizer]=cava
  [led-color]=openrgb
  [screenshot]=grim
  [region-select]=slurp
  [ocr]=tesseract
  [qr-scan]=zbar
  # Fedora ships wf-recorder directly. XDPH supplies hyprland-share-picker in
  # its own package, so neither feature relies on an Arch-only package name.
  [screen-record]=wf-recorder
  [screen-share-picker]=xdg-desktop-portal-hyprland
  [night-light]=hyprsunset
  [voice-type]=wtype
  [voice-stt]=voxtype-bin
  [media-control]=playerctl
  # the launcher's "@" live radio: mpv plays, mpv-mpris puts it on the players
  # bus (without it every now-playing surface goes blind to the broadcast),
  # yt-dlp resolves the YouTube /live pages at play time.
  [radio-player]=mpv
  [radio-mpris]=mpv-mpris
  [radio-resolver]=yt-dlp
  [brightness]=brightnessctl
  [idle]=hypridle
  [battery]=upower
  [shell]=quickshell
  [terminal]=kitty
  [browser]=chromium
  [files]=nautilus
  [editor]=neovim
  [file-cli]=yazi
  [video]=mpv
  [calc]=libqalculate
  [music-recognition]=songrec
  [display-brightness]=ddcutil
  [vibrance]=nvibrant-bin
  [network-kill-switch]=nftables
)

notreached=()
for feat in "${!need[@]}"; do
  pkg=${need[$feat]}
  [[ -n ${dependExempt[$pkg]:-} ]] && continue
  hard_depend "$pkg" || bundled "$pkg" || shipped_app "$pkg" || notreached+=("$feat -> $pkg")
done
if (( ${#notreached[@]} )); then
  echo "::error::feature tools no RPM delivers (not a ryoku-desktop hard depend, a first-party bundle, or a doctor-delivered app):" >&2
  printf '  %s\n' "${notreached[@]}" | sort >&2
  exit 1
fi

echo "shell-tool-availability: all ${#need[@]} gated feature packages reach every install path (ISO, update, shell installer)"
