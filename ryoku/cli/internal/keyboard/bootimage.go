package keyboard

import (
	"bufio"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// BootImageGlobs are where dracut writes the initramfs for the default
// GRUB/BLS layout: beside the kernel in /boot, which every user can stat.
var BootImageGlobs = []string{"/boot/initramfs-*.img"}

// UKIGlobs are where kernel-install's uki layout puts unified kernel images:
// EFI/Linux on the ESP (mounted at /efi, /boot or /boot/efi, per the Boot Loader
// Specification) or on an XBOOTLDR partition at /boot. Fedora mounts the ESP
// at /boot/efi as a vfat with dmask=0077, so a normal user cannot list it.
var UKIGlobs = []string{"/efi/EFI/Linux/*.efi", "/boot/EFI/Linux/*.efi", "/boot/efi/EFI/Linux/*.efi"}

// InstallConfPaths are kernel-install's config files, highest precedence
// first; the first one that exists is the one kernel-install reads.
var InstallConfPaths = []string{"/etc/kernel/install.conf", "/usr/lib/kernel/install.conf"}

// BootState is what can be said about the image the passphrase prompt boots
// from. Denied names a location this user could not read: the image there
// could not be checked, so neither stale nor current is claimed.
type BootState struct {
	Image  string
	Stale  bool
	Denied string
}

// Unknown reports whether the boot image could not be checked.
func (b BootState) Unknown() bool { return b.Denied != "" }

// BootImageTime is the newest non-rescue boot image, its mtime, and any image
// location this user could not read. The initramfs in /boot is what a GRUB box
// boots, so it answers unless kernel-install is set to the uki layout or there
// is none; then the unified kernel images on the ESP or XBOOTLDR answer.
func BootImageTime() (time.Time, string, string) {
	if kernelInstallLayout() != "uki" {
		if when, which, denied := newestBootImage(BootImageGlobs); which != "" || denied != "" {
			return when, which, denied
		}
	}
	return newestBootImage(UKIGlobs)
}

// newestBootImage scans globs for the newest image. A rescue image (dracut's
// initramfs-0-rescue-<id>.img, kernel-install's <id>-0-rescue.efi) is rebuilt
// on its own schedule, so it never speaks for freshness.
func newestBootImage(globs []string) (time.Time, string, string) {
	var newest time.Time
	var which, denied string
	for _, g := range globs {
		if _, err := os.ReadDir(filepath.Dir(g)); errors.Is(err, fs.ErrPermission) {
			if denied == "" {
				denied = filepath.Dir(g)
			}
			continue
		}
		paths, _ := filepath.Glob(g)
		for _, p := range paths {
			if strings.Contains(filepath.Base(p), "-0-rescue") {
				continue
			}
			fi, err := os.Stat(p)
			if err != nil {
				continue
			}
			if fi.ModTime().After(newest) {
				newest, which = fi.ModTime(), p
			}
		}
	}
	return newest, which, denied
}

// kernelInstallLayout is the layout= kernel-install is configured with, or ""
// when it is left to autodetect.
func kernelInstallLayout() string {
	for _, p := range InstallConfPaths {
		f, err := os.Open(p)
		if err != nil {
			continue
		}
		defer f.Close()
		layout := ""
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			if v, ok := strings.CutPrefix(strings.TrimSpace(sc.Text()), "layout="); ok {
				layout = strings.TrimSpace(v)
			}
		}
		return layout
	}
	return ""
}

// BootStale reports whether the boot image predates /etc/vconsole.conf, meaning
// the passphrase prompt still carries the keymap baked in when it was built.
// This is the check a plain config comparison misses: every file under /etc can
// agree while the prompt is still wrong.
func BootStale() BootState {
	when, path, denied := BootImageTime()
	if denied != "" {
		return BootState{Image: path, Denied: denied}
	}
	if when.IsZero() {
		return BootState{}
	}
	fi, err := os.Stat(VconsolePath)
	if err != nil {
		return BootState{Image: path}
	}
	return BootState{Image: path, Stale: fi.ModTime().After(when)}
}
