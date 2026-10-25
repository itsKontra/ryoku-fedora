#!/usr/bin/env bash
set -euo pipefail

PROFILE_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
REPO_ROOT=$(cd "$PROFILE_DIR/../.." && pwd)
TUI_DIR=$REPO_ROOT/installation/tui
BACKEND_DIR=$PROFILE_DIR/backend
SHARED_BACKEND=$REPO_ROOT/installation/backend/lib

OUT_DIR=${RYOKU_VOID_ISO_OUT:-$PROFILE_DIR/out}
WORK_DIR=${RYOKU_VOID_ISO_WORK:-$PROFILE_DIR/work}
STAGE_DIR=${RYOKU_VOID_ISO_STAGE:-$WORK_DIR/staging}
SPOOL_DIR=${RYOKU_VOID_ISO_SPOOL:-$WORK_DIR/spool}
ROOTFS=$STAGE_DIR/rootfs
CONTAINER_ENGINE=${RYOKU_CONTAINER_ENGINE:-docker}
CONTAINER_IMAGE=${RYOKU_VOID_ISO_IMAGE:-ghcr.io/void-linux/void-glibc-full:latest}
MKLIVE_REF=${RYOKU_VOID_MKLIVE_REF:-3aa194bdea9c83de0b5d78fb889a88f000a44fab}
ARCH=x86_64

log() { printf '\033[1;35m::\033[0m %s\n' "$*"; }
die() { printf 'void ISO build: error: %s\n' "$*" >&2; exit 1; }

usage() {
	printf 'usage: %s [--stage-only]\n' "${0##*/}" >&2
	exit 2
}

STAGE_ONLY=0
case ${1:-} in
	'') ;;
	--stage-only) STAGE_ONLY=1 ;;
	*) usage ;;
esac
[[ $# -le 1 ]] || usage

for command in git go tar; do
	command -v "$command" >/dev/null 2>&1 || die "required command not found: $command"
done
[[ -f $BACKEND_DIR/ryoku-install ]] || die "Void backend not found: $BACKEND_DIR/ryoku-install"
[[ -d $BACKEND_DIR/lib ]] || die "Void backend library not found: $BACKEND_DIR/lib"

if commit_epoch=$(git -C "$REPO_ROOT" log -1 --pretty=%ct 2>/dev/null) && [[ -n $commit_epoch ]]; then
	SOURCE_DATE_EPOCH=$commit_epoch
fi
SOURCE_DATE_EPOCH=${SOURCE_DATE_EPOCH:-$(date +%s)}
export SOURCE_DATE_EPOCH

PAYLOAD_COMMIT=$(git -C "$REPO_ROOT" rev-parse HEAD 2>/dev/null || printf unknown)
PAYLOAD_DATE=$(git -C "$REPO_ROOT" log -1 --pretty=%cI 2>/dev/null || date -Iseconds)
PAYLOAD_VERSION=$(tr -d '[:space:]' < "$REPO_ROOT/VERSION" 2>/dev/null || printf unknown)
PAYLOAD_NAME=$(tr -d '[:space:]' < "$REPO_ROOT/CODENAME" 2>/dev/null || printf unknown)
# Match the installed channel to its offline package source.
case ${RYOKU_VOID_ISO_CHANNEL:-${RYOKU_VOID_ISO_RYOKU_REPO:-}} in
	testing | */channels/testing/*) PAYLOAD_CHANNEL=testing ;;
	*) PAYLOAD_CHANNEL=stable ;;
esac
ISO_NAME=ryoku-void-${PAYLOAD_VERSION}-${ARCH}.iso

stage_repo() {
	local destination=$1
	mkdir -p "$destination"
	if [[ ${RYOKU_VOID_ISO_WORKTREE:-0} == 1 ]]; then
		log "Baking tracked and untracked, non-ignored working-tree files"
		(
			cd "$REPO_ROOT"
			git ls-files -co --exclude-standard -z \
				| LC_ALL=C sort -z \
				| tar --null --no-recursion --files-from=- -cf -
		) | tar -C "$destination" -xf -
	else
		git -C "$REPO_ROOT" archive --format=tar HEAD | tar -C "$destination" -xf -
	fi
}

log "Staging fresh Void live root at $ROOTFS"
rm -rf "$STAGE_DIR"
mkdir -p "$ROOTFS"
cp -a "$PROFILE_DIR/rootfs"/. "$ROOTFS/"

log "Building static ryoku-tui"
install -d "$ROOTFS/usr/local/bin"
(
	cd "$TUI_DIR"
	CGO_ENABLED=0 go build -trimpath -ldflags '-s -w -buildid=' \
		-o "$ROOTFS/usr/local/bin/ryoku-tui" .
)

log "Staging Void installer backend"
install -d "$ROOTFS/usr/local/lib/ryoku/backend/lib" "$ROOTFS/usr/local/lib/ryoku/backend/shared"
install -m0755 "$BACKEND_DIR/ryoku-install" "$ROOTFS/usr/local/lib/ryoku/backend/ryoku-install"
cp -a "$BACKEND_DIR/lib"/. "$ROOTFS/usr/local/lib/ryoku/backend/lib/"
for library in common i18n choices preflight disk resize luks filesystem accounts snapshots network deploy lanes dracut; do
	install -m0644 "$SHARED_BACKEND/$library.sh" \
		"$ROOTFS/usr/local/lib/ryoku/backend/shared/$library.sh"
done

# Void adds only login and runit wiring around the shared graphical session.
install -d "$ROOTFS/usr/local/lib/ryoku"
install -m0755 "$REPO_ROOT/installation/iso/airootfs/usr/local/bin/ryoku-installer-session" \
	"$ROOTFS/usr/local/lib/ryoku/installer-session"

log "Baking repository payload"
stage_repo "$ROOTFS/usr/share/ryoku"
install -d "$ROOTFS/usr/share/ryoku/installation/installerplan"
install -m0644 "$REPO_ROOT/installation/installerplan/choices.tsv" \
	"$ROOTFS/usr/share/ryoku/installation/installerplan/choices.tsv"
cat > "$ROOTFS/usr/share/ryoku/.payload" <<EOF
commit=$PAYLOAD_COMMIT
date=$PAYLOAD_DATE
version=$PAYLOAD_VERSION
name=$PAYLOAD_NAME
channel=$PAYLOAD_CHANNEL
EOF
printf 'void\n' > "$ROOTFS/usr/share/ryoku/variant"

log "Installing translation catalog"
install -d "$ROOTFS/usr/share/ryoku/i18n"
install -m0644 "$REPO_ROOT"/ryoku/i18n/catalog/*.json "$ROOTFS/usr/share/ryoku/i18n/"
install -m0644 "$REPO_ROOT/ryoku/i18n/langs.json" "$ROOTFS/usr/share/ryoku/i18n/langs.json"
sed -i \
	-e "s|@RYOKU_NAME@|$PAYLOAD_NAME|g" \
	-e "s|@RYOKU_VERSION@|$PAYLOAD_VERSION|g" \
	-e "s|@RYOKU_COMMIT@|${PAYLOAD_COMMIT:0:12}|g" \
	"$ROOTFS/etc/motd"
chmod 0755 \
	"$ROOTFS/usr/local/bin/ryoku-installer-session" \
	"$ROOTFS/usr/local/bin/ryoku-install" \
	"$ROOTFS/usr/local/bin/ryoku-tui" \
	"$ROOTFS/usr/local/lib/ryoku/installer-session" \
	"$ROOTFS/usr/local/lib/ryoku/backend/ryoku-install"

log "Staged rootfs at $ROOTFS"
if ((STAGE_ONLY)); then
	log "--stage-only: skipping the offline repository and void-mklive"
	exit 0
fi

command -v "$CONTAINER_ENGINE" >/dev/null 2>&1 \
	|| die "$CONTAINER_ENGINE is required for the privileged void-mklive build"
for command in clang lld-link; do
	command -v "$command" >/dev/null 2>&1 || die "required command not found: $command"
done
RYOKU_REPO=${RYOKU_VOID_ISO_RYOKU_REPO:-}
[[ -n $RYOKU_REPO ]] \
	|| die "set RYOKU_VOID_ISO_RYOKU_REPO to a built Ryoku XBPS repository path or URL"
[[ ${RYOKU_VOID_ISO_OFFLINE_SKIP:-0} != 1 ]] \
	|| die "RYOKU_VOID_ISO_OFFLINE_SKIP is only valid with --stage-only"

mkdir -p "$OUT_DIR" "$WORK_DIR" "$SPOOL_DIR"
OUT_DIR=$(cd "$OUT_DIR" && pwd -P)
WORK_DIR=$(cd "$WORK_DIR" && pwd -P)
SPOOL_DIR=$(cd "$SPOOL_DIR" && pwd -P)
STAGE_DIR=$(cd "$STAGE_DIR" && pwd -P)
rm -f "$OUT_DIR/$ISO_NAME" "$OUT_DIR/SHA256SUMS"

log "Building UEFI live-menu actions"
EFI_ACTION_DIR=$STAGE_DIR/efi-actions
EFI_ACTION_OBJECT=$EFI_ACTION_DIR/action.obj
mkdir -p "$EFI_ACTION_DIR"
for action in restart shutdown; do
	case $action in
		restart) reset_type=0 ;;
		shutdown) reset_type=2 ;;
	esac
	clang --target=x86_64-pc-windows-msvc \
		-ffreestanding -fshort-wchar -mno-red-zone -fno-stack-protector \
		-DRYOKU_RESET_TYPE="$reset_type" \
		-c "$PROFILE_DIR/efi-action.c" -o "$EFI_ACTION_OBJECT"
	lld-link /subsystem:efi_application /entry:efi_main /nodefaultlib \
		/machine:x64 /dynamicbase /fixed:no \
		"/out:$EFI_ACTION_DIR/$action.efi" "$EFI_ACTION_OBJECT"
done
rm -f "$EFI_ACTION_OBJECT"



run=("$CONTAINER_ENGINE" run --rm --platform linux/amd64 --privileged
	-v /dev:/dev
	-v "$REPO_ROOT:/repo:ro"
	-v "$WORK_DIR:/work"
	-v "$SPOOL_DIR:/spool"
	-v "$OUT_DIR:/out"
	-v "$STAGE_DIR:/stage"
	-w /work
	-e "SOURCE_DATE_EPOCH=$SOURCE_DATE_EPOCH"
	-e "RYOKU_VOID_MKLIVE_REF=$MKLIVE_REF"
	-e RYOKU_VOID_ISO_KEYS_OUT=/spool/keys
	-e RYOKU_VOID_ISO_OFFLINE_WORK=/spool/offline-work
	-e RYOKU_ISO_ROOT_REPO=/spool/offline-repo
	-e "RYOKU_ISO_NAME=$ISO_NAME"
	-e "RYOKU_ISO_TITLE=Ryoku Void $PAYLOAD_VERSION"
	-e "HOST_UID=$(id -u)"
	-e "HOST_GID=$(id -g)")

if [[ -d $RYOKU_REPO ]]; then
	RYOKU_REPO=$(cd "$RYOKU_REPO" && pwd -P)
	run+=( -v "$RYOKU_REPO:/ryoku-repo:ro" )
	RYOKU_REPO=/ryoku-repo
fi
run+=( -e "RYOKU_VOID_ISO_RYOKU_REPO=$RYOKU_REPO" )
if [[ -n ${RYOKU_VOID_ISO_KEYRING_DIR:-} ]]; then
	[[ -d $RYOKU_VOID_ISO_KEYRING_DIR ]] \
		|| die "RYOKU_VOID_ISO_KEYRING_DIR is not a directory: $RYOKU_VOID_ISO_KEYRING_DIR"
	KEYRING_DIR=$(cd "$RYOKU_VOID_ISO_KEYRING_DIR" && pwd -P)
	run+=( -v "$KEYRING_DIR:/ryoku-keys:ro" -e RYOKU_VOID_ISO_KEYRING_DIR=/ryoku-keys )
else
	run+=( -e RYOKU_VOID_ISO_KEYRING_DIR= )
fi

readarray -t live_packages < <(awk '!/^[[:space:]]*(#|$)/ { print $1 }' "$PROFILE_DIR/packages.x86_64")
printf -v LIVE_PACKAGES '%s ' "${live_packages[@]}"
run+=( -e "RYOKU_LIVE_PACKAGES=${LIVE_PACKAGES% }" "$CONTAINER_IMAGE" )

inner=$(cat <<'INNER'
set -euo pipefail
RAW_ISO=/spool/$RYOKU_ISO_NAME.raw
FINAL_ISO=/work/$RYOKU_ISO_NAME.final
restore_owner() {
	chown -R "$HOST_UID:$HOST_GID" /out /work /spool /stage 2>/dev/null || true
}
cleanup() {
	rm -f "$RAW_ISO" "$FINAL_ISO"
	restore_owner
}
trap cleanup EXIT

xbps-install -Suy xbps >/dev/null
xbps-install -y bash curl ca-certificates patch tar xz lzo kmod outils dosfstools e2fsprogs xorriso >/dev/null
mkdir -p /verify
mount -t tmpfs -o size=64g,mode=0755 ryoku-verify /verify
export RYOKU_VOID_ISO_VERIFY_ROOT=/verify


rm -rf /spool/offline-repo /spool/offline-work /spool/keys
mkdir -p /spool/offline-repo /spool/keys
/repo/void/iso/offline-repo.sh /repo /spool/offline-repo
umount /verify
rmdir /verify
unset RYOKU_VOID_ISO_VERIFY_ROOT

rm -rf /work/limine-root
mkdir -p /work/limine-root/var/db/xbps/keys
if compgen -G '/var/db/xbps/keys/*.plist' >/dev/null; then
	cp -a /var/db/xbps/keys/*.plist /work/limine-root/var/db/xbps/keys/
fi
if compgen -G '/spool/keys/*.plist' >/dev/null; then
	cp -a /spool/keys/*.plist /work/limine-root/var/db/xbps/keys/
fi
XBPS_ARCH=x86_64 xbps-install \
	-r /work/limine-root -i -R /spool/offline-repo -Sy limine >/dev/null
rm -rf /spool/offline-work

rm -rf /work/void-mklive /work/mklive-source
mkdir -p /work/mklive-source /work/mklive-root
curl -fsSL --retry 3 \
	"https://github.com/void-linux/void-mklive/archive/${RYOKU_VOID_MKLIVE_REF}.tar.gz" \
	| tar -xz -C /work/mklive-source --strip-components=1
cp -a /work/mklive-source/. /work/void-mklive
patch -d /work/void-mklive -p1 < /repo/void/iso/mklive-root-repo.patch

if compgen -G '/spool/keys/*.plist' >/dev/null; then
	mkdir -p /stage/rootfs/var/db/xbps/keys /work/void-mklive/keys
	cp -a /spool/keys/*.plist /stage/rootfs/var/db/xbps/keys/
	cp -a /spool/keys/*.plist /work/void-mklive/keys/
fi

rm -f "$RAW_ISO"
rm -rf /work/mklive-root
mkdir -p /work/mklive-root
cd /work/void-mklive
	# Ryoku's live TUI font must resolve from the package closure.
RYOKU_MKLIVE_DROP_PATHS='/spool/xbps-cache-target /spool/xbps-cache-host' \
ROOTDIR=/work/mklive-root \
	./mklive.sh \
	-a x86_64 \
	-b base-system \
	-c /spool/xbps-cache-target \
	-H /spool/xbps-cache-host \
	-r /spool/offline-repo \
	-p "$RYOKU_LIVE_PACKAGES" \
	-S 'dbus NetworkManager seatd' \
	-I /stage/rootfs \
	-x /repo/void/iso/post-setup.sh \
	-e /bin/bash \
	-T "$RYOKU_ISO_TITLE" \
	-C 'console=tty1 console=ttyS0,115200n8' \
	-o "$RAW_ISO"

cd /
rm -rf \
	/work/mklive-root \
	/work/void-mklive \
	/work/mklive-source \
	/spool/xbps-cache-target \
	/spool/xbps-cache-host \
	/spool/offline-repo \
	/spool/offline-work \
	/spool/keys
/repo/void/iso/install-limine.sh \
	"$RAW_ISO" "$FINAL_ISO" "$RYOKU_ISO_TITLE" \
	/work/limine-root /stage/efi-actions
rm -f "$RAW_ISO"
mv "$FINAL_ISO" "/out/$RYOKU_ISO_NAME"
rm -rf /work/limine-root

cleanup
trap - EXIT
INNER
)

log "Building offline closure and ISO in $CONTAINER_IMAGE"
"${run[@]}" sh -c \
	'set -e; xbps-install -Syu xbps >/dev/null || xbps-install -yu xbps >/dev/null; xbps-install -y bash >/dev/null; exec bash -euo pipefail -c "$1"' \
	ryoku-void-iso "$inner"

[[ -s $OUT_DIR/$ISO_NAME ]] || die "ISO build did not produce $OUT_DIR/$ISO_NAME"
(
	cd "$OUT_DIR"
	sha256sum -- "$ISO_NAME" > SHA256SUMS
)
log "ISO and SHA256SUMS written to $OUT_DIR"
