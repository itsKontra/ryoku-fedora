#!/usr/bin/env bash
#
# ryoku-shell bootstrap: fetch and run the standalone Ryoku desktop installer
# on an existing Arch machine. Kept deliberately dumb: every real decision
# lives in the ryoku-shell-install binary this script downloads.
#
#   curl -fsSL https://raw.githubusercontent.com/itsKontra/ryoku-fedora/main/ryoku-shell-installer/install.sh | bash
#
# args after `bash -s --` are forwarded to the installer (--yes, --dry-run).
# RYOKU_SHELL_REF picks the git ref to fetch the installer and payload from.
set -euo pipefail

work=""
cleanup() {
  if [[ -n "$work" && -d "$work" ]]; then
    rm -rf "$work"
  fi
}
trap cleanup EXIT

main() {
  local current_branch=""
  if git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
    current_branch="$(git rev-parse --abbrev-ref HEAD 2>/dev/null || true)"
  fi
  local ref="${RYOKU_SHELL_REF:-${current_branch:-main}}"
  local repo="${RYOKU_SHELL_REPO:-https://github.com/itsKontra/ryoku-fedora.git}"
  local ref_specified=0
  [[ -n "${RYOKU_SHELL_REF:-}" ]] && ref_specified=1
  local args=("$@") i
  for ((i=0; i<${#args[@]}; i++)); do
    case "${args[i]}" in
      --ref) i=$((i+1)); ref="${args[i]:?--ref needs a value}"; ref_specified=1 ;;
      --ref=*) ref="${args[i]#--ref=}"; ref_specified=1 ;;
      --repo) i=$((i+1)); repo="${args[i]:?--repo needs a value}" ;;
      --repo=*) repo="${args[i]#--repo=}" ;;
    esac
  done
  [[ $repo == https://github.com/*/* ]] || { echo 'bootstrap requires an HTTPS GitHub repository URL' >&2; return 1; }
  local slug="${repo#https://github.com/}"
  slug="${slug%.git}"
  local raw="https://raw.githubusercontent.com/${slug}/${ref}/ryoku-shell-installer"

  # English on purpose: this bootstrap runs before any Ryoku catalog exists on
  # the box to translate from; the ryoku-shell-install binary it fetches does that.
  say() { printf '\033[38;2;242;86;35m==>\033[0m %s\n' "$*"; }
  die() {
    printf 'ryoku-shell: %s\n' "$*" >&2
    exit 1
  }

  [[ $(id -u) -ne 0 ]] || die "run as your normal user, not root (sudo is used when needed)"

  # NixOS needs a nix-based engine; that work is parked (archived flake),
  # so refuse honestly instead of dying on the package-manager guard below.
  [[ ! -e /run/ostree-booted ]] || die "immutable Fedora derivatives are not supported"

  [[ ! -e /etc/NIXOS ]] || die "NixOS is not supported yet; use the flake instead"

  local ryoku_family
  if command -v pacman > /dev/null 2>&1; then
    ryoku_family=arch
  elif command -v apt-get > /dev/null 2>&1; then
    ryoku_family=debian
  elif command -v dnf > /dev/null 2>&1; then
    ryoku_family=fedora
  else
    die "unsupported distribution: Ryoku installs on Arch-based, Debian-based, and Fedora-based systems"
  fi
  [[ $(uname -m) == x86_64 ]] || die "Ryoku ships x86_64 builds only"
  # the binary refuses non-systemd boots much later (session + services are
  # systemd units); saying it here spares Artix users the download.
  [[ -d /run/systemd/system ]] || die "this installer needs systemd (Artix and other non-systemd inits are not supported)"
  command -v curl > /dev/null 2>&1 || die "curl is required"

  # warn-only on derivatives: the package manager is what actually matters.
  if [[ -r /etc/os-release ]]; then
    # shellcheck source=/dev/null
    . /etc/os-release
    case "${ID:-} ${ID_LIKE:-}" in
      *arch*|*debian*|*fedora*) ;;
      *) say "warning: ${PRETTY_NAME:-unknown distro} is not recognised; continuing as ${ryoku_family}" ;;
    esac
  fi
  if [[ $ryoku_family == debian ]]; then
    say "Debian detected: the desktop is built from source, which takes a few minutes"
  elif [[ $ryoku_family == fedora ]]; then
    say "Fedora detected: released packages are the default; --install-mode=source builds a checkout"
  fi

  local here=""
  if [[ -n "${BASH_SOURCE[0]:-}" && -f "${BASH_SOURCE[0]}" ]]; then
    here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
  fi

  local installer_bin=""
  if [[ -n "$here" && -x "$here/ryoku-shell-install" && $ref_specified -eq 0 ]]; then
    installer_bin="$here/ryoku-shell-install"
  else
    work="$(mktemp -d)"
    say "fetching the Ryoku shell installer (${ref})"
    curl -fsSL --retry 3 -o "$work/ryoku-shell-install" "$raw/ryoku-shell-install"
    curl -fsSL --retry 3 -o "$work/ryoku-shell-install.sha256" "$raw/ryoku-shell-install.sha256"
    (cd "$work" && sha256sum --check --quiet ryoku-shell-install.sha256) \
      || die "checksum mismatch on the downloaded installer; try again"
    chmod +x "$work/ryoku-shell-install"
    installer_bin="$work/ryoku-shell-install"
  fi

  say "starting the installer"
  local rc=0
  # piped stdin (curl | bash) is useless to a TUI; hand it the real terminal.
  if [[ ! -t 0 && -r /dev/tty ]]; then
    RYOKU_SHELL_REPO="$repo" RYOKU_SHELL_REF="$ref" "$installer_bin" "$@" < /dev/tty || rc=$?
  else
    RYOKU_SHELL_REPO="$repo" RYOKU_SHELL_REF="$ref" "$installer_bin" "$@" || rc=$?
  fi
  return "$rc"
}

main "$@"
