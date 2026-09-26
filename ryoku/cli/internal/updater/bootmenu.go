package updater

import (
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"ryoku-cli/internal/sys"
)

// The snapshot boot menu makes snapper snapshots bootable from Fedora's GRUB
// without touching what Secure Boot verifies (issue #48). Pieces:
//
//   - /etc/grub.d/42_ryoku_snapshots, rendered into grub.cfg once, sources
//     snapshotMenuCfg, which `ryoku boot-menu sync` rewrites after every
//     snapshot (the snapper plugin starts ryoku-snapshot-menu.service).
//   - GRUB cannot read the root filesystem on an encrypted install, so each
//     kernel version a snapshot needs gets a copy on /boot under
//     snapshotKernelDir: Fedora's signed vmlinuz, which shim and GRUB verify
//     like any other kernel, and an initramfs built with the ryoku-snapshot
//     dracut module.
//   - Each snapshot gets two entries. Look boots it read-only with writes in
//     RAM (rd.ryoku.preview). Restore makes it the root subvolume again from
//     the initramfs, after LUKS is unlocked, and keeps the replaced root as
//     <root>.broken-<time> (rd.ryoku.restore).
//   - The first boot after a restore runs `ryoku boot-menu restored`: /boot is
//     not in the snapshot, so its entries are brought in line with the kernels
//     the restored root has.
const (
	snapshotMenuID     = "ryoku-snapshots"
	snapshotMenuScript = "/etc/grub.d/42_ryoku_snapshots"
	snapshotDracutMod  = "ryoku-snapshot"
	restoredFlag       = "/run/ryoku/restored"
	// a set-aside root is deleted this long after a restore; the newest one
	// is always kept.
	brokenRootKeep = 14 * 24 * time.Hour
	// what a new kernel copy must leave free on /boot, so a Fedora kernel
	// update still fits after it.
	bootHeadroom = 300 << 20
)

// Paths the menu reads and writes, variables so tests can point them at a
// scratch tree.
var (
	bootDir         = "/boot"
	snapshotsDir    = "/.snapshots"
	modulesDir      = "/usr/lib/modules"
	blsDir          = "/boot/loader/entries"
	kernelCmdline   = "/etc/kernel/cmdline"
	mountInfo       = "/proc/self/mountinfo"
	dracutModuleDir = "/usr/lib/dracut/modules.d/90ryoku-snapshot"
)

func snapshotMenuCfg() string   { return filepath.Join(bootDir, "grub2", "ryoku-snapshots.cfg") }
func snapshotKernelDir() string { return filepath.Join(bootDir, "ryoku", "snapshots") }

// BootMenu is `ryoku boot-menu`, run as root by the snapshot units.
func BootMenu(args []string) error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("ryoku boot-menu runs as root (ryoku-snapshot-menu.service)")
	}
	switch {
	case len(args) == 1 && args[0] == "sync":
		return syncSnapshotMenu()
	case len(args) == 1 && args[0] == "restored":
		return snapshotRestored()
	}
	return fmt.Errorf("usage: ryoku boot-menu sync|restored")
}

// bootSnapshot is one snapper snapshot as the menu shows it.
type bootSnapshot struct {
	Num         string `xml:"num"`
	Kind        string `xml:"type"`
	Date        string `xml:"date"`
	Description string `xml:"description"`
	kver        string
}

// menuLayout is what every entry shares: where /boot is for GRUB, the kernel
// options, and the btrfs subvolume names the cmdlines use.
type menuLayout struct {
	bootUUID   string
	options    string
	rootflags  []string
	rootSubvol string
	snapSubvol string
}

func syncSnapshotMenu() error {
	layout, err := readMenuLayout()
	if err != nil {
		fmt.Println("boot menu:", err)
		return nil
	}
	snaps := readBootSnapshots(snapshotsDir)
	var listed []bootSnapshot
	used := map[string]bool{}
	for _, s := range snaps {
		if s.kver == "" {
			continue
		}
		if err := ensureSnapshotKernel(s.kver); err != nil {
			fmt.Printf("boot menu: snapshot %s left out: %v\n", s.Num, err)
			continue
		}
		used[s.kver] = true
		listed = append(listed, s)
	}
	if err := writeFileAtomic(snapshotMenuCfg(), renderSnapshotMenu(listed, layout), 0o644); err != nil {
		return err
	}
	pruneSnapshotKernels(used)
	pruneBrokenRoots(layout)
	fmt.Printf("boot menu: %d snapshot(s) in the Ryoku snapshots menu\n", len(listed))
	return nil
}

// readMenuLayout gathers the facts the entries need and refuses a layout the
// restore cannot serve: /boot must be its own filesystem (GRUB reads the
// kernel copies from it), and the snapshots must live outside the root
// subvolume, since a restore renames that subvolume.
func readMenuLayout() (menuLayout, error) {
	var l menuLayout
	mounts := parseMountInfo(readText(mountInfo))
	boot, ok := mounts[bootDir]
	if !ok {
		return l, fmt.Errorf("/boot is not a separate filesystem; snapshots are not bootable")
	}
	root, ok := mounts["/"]
	if !ok || root.fstype != "btrfs" {
		return l, fmt.Errorf("/ is not btrfs; no snapshots to boot")
	}
	snaps, ok := mounts[snapshotsDir]
	if !ok || snaps.fstype != "btrfs" {
		return l, fmt.Errorf("%s is not its own btrfs subvolume; snapshots are not bootable", snapshotsDir)
	}
	l.rootSubvol = strings.Trim(root.root, "/")
	l.snapSubvol = strings.Trim(snaps.root, "/")
	if l.rootSubvol == "" || strings.Contains(l.rootSubvol, "/") || strings.HasPrefix(l.snapSubvol, l.rootSubvol+"/") {
		return l, fmt.Errorf("the snapshots live inside the root subvolume; a restore cannot replace it")
	}
	out, err := sys.RunOut("blkid", "-s", "UUID", "-o", "value", boot.source)
	if l.bootUUID = strings.TrimSpace(out); err != nil || l.bootUUID == "" {
		return l, fmt.Errorf("no filesystem UUID for /boot (%s)", boot.source)
	}
	base := strings.TrimSpace(readText(kernelCmdline))
	if base == "" {
		base = strings.TrimSpace(readText("/proc/cmdline"))
	}
	l.options, l.rootflags = baseSnapshotOptions(base)
	if !strings.Contains(" "+l.options, " root=") {
		return l, fmt.Errorf("no root= in the kernel command line")
	}
	return l, nil
}

type mountEntry struct {
	root, fstype, source string
}

// parseMountInfo maps each mount point to its btrfs subvolume path (the
// mountinfo root field), filesystem type and source device.
func parseMountInfo(text string) map[string]mountEntry {
	out := map[string]mountEntry{}
	for _, line := range strings.Split(text, "\n") {
		pre, post, ok := strings.Cut(line, " - ")
		if !ok {
			continue
		}
		f, g := strings.Fields(pre), strings.Fields(post)
		if len(f) < 5 || len(g) < 2 {
			continue
		}
		out[f[4]] = mountEntry{root: f[3], fstype: g[0], source: g[1]}
	}
	return out
}

// baseSnapshotOptions splits a kernel command line into the options every
// entry repeats and the rootflags besides subvol=, dropping what each entry
// sets itself: the image, ro/rw, the subvolume, and earlier Ryoku or unit
// overrides.
func baseSnapshotOptions(cmdline string) (opts string, rootflags []string) {
	var keep []string
	for _, tok := range strings.Fields(cmdline) {
		switch {
		case tok == "ro", tok == "rw",
			strings.HasPrefix(tok, "BOOT_IMAGE="),
			strings.HasPrefix(tok, "systemd.unit="),
			strings.HasPrefix(tok, "rd.ryoku."):
		case strings.HasPrefix(tok, "rootflags="):
			for _, o := range strings.Split(strings.TrimPrefix(tok, "rootflags="), ",") {
				if o != "" && !strings.HasPrefix(o, "subvol=") && !strings.HasPrefix(o, "subvolid=") {
					rootflags = append(rootflags, o)
				}
			}
		default:
			keep = append(keep, tok)
		}
	}
	return strings.Join(keep, " "), rootflags
}

// entryOptions is the full cmdline of one entry: the shared options, the root
// mounted read-only from subvol, then the Ryoku argument.
func (l menuLayout) entryOptions(subvol, arg string) string {
	flags := strings.Join(append([]string{"subvol=" + subvol}, l.rootflags...), ",")
	return strings.TrimSpace(l.options + " ro rootflags=" + flags + " " + arg)
}

// readBootSnapshots lists the snapshots newest first, each with the newest
// kernel version its root carries.
func readBootSnapshots(dir string) []bootSnapshot {
	entries, _ := os.ReadDir(dir)
	var out []bootSnapshot
	for _, e := range entries {
		if _, err := strconv.Atoi(e.Name()); err != nil || e.Name() == "0" {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, e.Name(), "info.xml"))
		if err != nil {
			continue
		}
		var s bootSnapshot
		if xml.Unmarshal(raw, &s) != nil || s.Num != e.Name() {
			continue
		}
		s.kver = newestKernel(filepath.Join(dir, e.Name(), "snapshot", "usr/lib/modules"))
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool {
		a, _ := strconv.Atoi(out[i].Num)
		b, _ := strconv.Atoi(out[j].Num)
		return a > b
	})
	return out
}

// newestKernel is the highest kernel version under a modules directory that
// has its vmlinuz, the one that root boots by default.
func newestKernel(dir string) string {
	entries, _ := os.ReadDir(dir)
	best := ""
	for _, e := range entries {
		if !sys.Exists(filepath.Join(dir, e.Name(), "vmlinuz")) {
			continue
		}
		if best == "" || compareVersions(e.Name(), best) > 0 {
			best = e.Name()
		}
	}
	return best
}

// compareVersions orders kernel versions the way rpm does for the cases
// Fedora produces: runs of digits compare as numbers, anything else as text.
func compareVersions(a, b string) int {
	split := func(s string) []string {
		var parts []string
		for len(s) > 0 {
			i := 0
			digit := unicode.IsDigit(rune(s[0]))
			for i < len(s) && unicode.IsDigit(rune(s[i])) == digit {
				i++
			}
			parts = append(parts, s[:i])
			s = s[i:]
		}
		return parts
	}
	pa, pb := split(a), split(b)
	for i := 0; i < len(pa) && i < len(pb); i++ {
		x, errx := strconv.Atoi(pa[i])
		y, erry := strconv.Atoi(pb[i])
		switch {
		case errx == nil && erry == nil && x != y:
			if x < y {
				return -1
			}
			return 1
		case (errx != nil || erry != nil) && pa[i] != pb[i]:
			return strings.Compare(pa[i], pb[i])
		}
	}
	return len(pa) - len(pb)
}

// ensureSnapshotKernel keeps a bootable copy of kver on /boot. The initramfs
// is built from the running system, so it needs kver's modules installed; a
// copy made while they were is kept after the kernel is removed. It is
// rebuilt when the dracut module changes and the kernel is still around.
func ensureSnapshotKernel(kver string) error {
	dir := filepath.Join(snapshotKernelDir(), kver)
	vmlinuz, initrd := filepath.Join(dir, "vmlinuz"), filepath.Join(dir, "initramfs.img")
	src := filepath.Join(modulesDir, kver, "vmlinuz")
	have := sys.Exists(vmlinuz) && sys.Exists(initrd)
	installed := sys.Exists(src)
	if have && (!installed || !newerThan(dracutModuleDir, initrd)) {
		return nil
	}
	if !installed {
		return fmt.Errorf("kernel %s is no longer installed and has no copy on /boot", kver)
	}
	if free, ok := sys.FreeBytes(bootDir); ok && !have {
		need := fileSize(src) + fileSize(filepath.Join(bootDir, "initramfs-"+kver+".img")) + bootHeadroom
		if free < uint64(need) {
			return fmt.Errorf("/boot has %d MiB free, too little for a copy of kernel %s", free>>20, kver)
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := sys.CopyFile(src, vmlinuz+".new"); err != nil {
		return err
	}
	build := exec.Command("dracut", "--quiet", "--force", "--add", snapshotDracutMod, "--kver", kver, initrd+".new")
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		_ = os.Remove(vmlinuz + ".new")
		_ = os.Remove(initrd + ".new")
		return fmt.Errorf("building the initramfs for %s: %w", kver, err)
	}
	if err := os.Rename(vmlinuz+".new", vmlinuz); err != nil {
		return err
	}
	return os.Rename(initrd+".new", initrd)
}

// newerThan reports whether any file under dir changed after path did.
func newerThan(dir, path string) bool {
	ref, err := os.Stat(path)
	if err != nil {
		return true
	}
	newer := false
	_ = filepath.WalkDir(dir, func(_ string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if fi, err := d.Info(); err == nil && fi.ModTime().After(ref.ModTime()) {
				newer = true
			}
		}
		return nil
	})
	return newer
}

func fileSize(path string) int64 {
	if fi, err := os.Stat(path); err == nil {
		return fi.Size()
	}
	return 0
}

// pruneSnapshotKernels drops the copies no listed snapshot boots.
func pruneSnapshotKernels(used map[string]bool) {
	entries, _ := os.ReadDir(snapshotKernelDir())
	for _, e := range entries {
		if !used[e.Name()] {
			_ = os.RemoveAll(filepath.Join(snapshotKernelDir(), e.Name()))
		}
	}
}

// snapshotEntryID is the GRUB id of a snapshot's look or restore entry;
// grub2-reboot takes it as "<menu>><entry>".
func snapshotEntryID(num, action string) string {
	return snapshotMenuID + ">" + "ryoku-snapshot-" + num + "-" + action
}

// renderSnapshotMenu is the whole ryoku-snapshots.cfg.
func renderSnapshotMenu(snaps []bootSnapshot, l menuLayout) string {
	var b strings.Builder
	b.WriteString("# Written by `ryoku boot-menu sync` after every snapshot; edits are lost.\n")
	if len(snaps) == 0 {
		return b.String()
	}
	fmt.Fprintf(&b, "submenu 'Ryoku snapshots' --id %s {\n", snapshotMenuID)
	for _, s := range snaps {
		snap := l.snapSubvol + "/" + s.Num + "/snapshot"
		label := snapshotLabel(s)
		entry := func(title, action, subvol, arg string) {
			fmt.Fprintf(&b, "  menuentry '%s' --id ryoku-snapshot-%s-%s {\n", grubQuote(title), s.Num, action)
			fmt.Fprintf(&b, "    search --no-floppy --fs-uuid --set=root %s\n", l.bootUUID)
			fmt.Fprintf(&b, "    linux /ryoku/snapshots/%s/vmlinuz %s\n", s.kver, l.entryOptions(subvol, arg))
			fmt.Fprintf(&b, "    initrd /ryoku/snapshots/%s/initramfs.img\n", s.kver)
			b.WriteString("  }\n")
		}
		entry(label+": look (read-only)", "preview", snap, "rd.ryoku.preview="+snap)
		entry(label+": restore", "restore", l.rootSubvol, "rd.ryoku.restore="+snap)
	}
	b.WriteString("}\n")
	return b.String()
}

// snapshotLabel: "Snapshot 42, 2026-09-20 10:11, pre ryoku update".
func snapshotLabel(s bootSnapshot) string {
	label := "Snapshot " + s.Num + ", " + shortSnapDate(s.Date) + " UTC"
	if d := strings.TrimSpace(s.Kind + " " + s.Description); d != "" {
		label += ", " + d
	}
	return label
}

// grubQuote makes text safe inside GRUB single quotes, which cannot hold a
// quote at all; control characters go too.
func grubQuote(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\'' || unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
}

// pruneBrokenRoots deletes the roots earlier restores set aside once they are
// older than brokenRootKeep, keeping the newest regardless.
func pruneBrokenRoots(l menuLayout) {
	top, cleanup, err := mountBtrfsTop()
	if err != nil {
		return
	}
	defer cleanup()
	matches, _ := filepath.Glob(filepath.Join(top, l.rootSubvol+".broken-*"))
	sort.Strings(matches)
	for i, m := range matches {
		if i == len(matches)-1 {
			break
		}
		stamp := strings.TrimPrefix(filepath.Base(m), l.rootSubvol+".broken-")
		at, err := time.Parse("20060102-150405", stamp)
		if err != nil || time.Since(at) < brokenRootKeep {
			continue
		}
		if err := sys.Run("btrfs", "subvolume", "delete", m); err == nil {
			fmt.Printf("boot menu: deleted %s, set aside by a restore on %s\n", filepath.Base(m), at.Format("2006-01-02"))
		}
	}
}

// mountBtrfsTop mounts the top level of the root filesystem privately under
// /run/ryoku, where the set-aside roots live next to the root subvolume.
func mountBtrfsTop() (string, func(), error) {
	root, ok := parseMountInfo(readText(mountInfo))["/"]
	if !ok {
		return "", nil, fmt.Errorf("no root mount")
	}
	top := "/run/ryoku/btrfs-top"
	if err := os.MkdirAll(top, 0o700); err != nil {
		return "", nil, err
	}
	if err := sys.Run("mount", "-t", "btrfs", "-o", "subvolid=5", root.source, top); err != nil {
		return "", nil, err
	}
	return top, func() { _ = sys.Run("umount", top) }, nil
}

// snapshotRestored runs on the first boot of a restored root. Fedora's BLS
// entries for kernels the restored root has no modules for are removed, the
// restored root's kernels get entries (kernel-install also builds their
// initramfs and the console twins), and the kernel running now, the one the
// snapshot booted with, becomes the default.
func snapshotRestored() error {
	what := strings.Fields(readText(restoredFlag))
	if len(what) < 2 {
		return nil
	}
	fmt.Printf("boot menu: this root was restored from %s; the one it replaced is %s\n", what[0], what[1])
	mid := strings.TrimSpace(readText("/etc/machine-id"))
	for _, kver := range blsKernels(blsDir, mid) {
		if !sys.Exists(filepath.Join(modulesDir, kver)) {
			_ = sys.Run("kernel-install", "remove", kver)
		}
	}
	modules, _ := os.ReadDir(modulesDir)
	for _, m := range modules {
		kver := m.Name()
		img := filepath.Join(modulesDir, kver, "vmlinuz")
		if sys.Exists(img) && !sys.Exists(filepath.Join(blsDir, mid+"-"+kver+".conf")) {
			_ = sys.Run("kernel-install", "add", kver, img)
		}
	}
	if out, err := sys.RunOut("uname", "-r"); err == nil {
		if img := filepath.Join(bootDir, "vmlinuz-"+strings.TrimSpace(out)); sys.Exists(img) {
			_ = sys.Run("grubby", "--set-default", img)
		}
	}
	_ = writeNotice(bootNotice{Action: "snapshot-restored", Snapshot: what[0],
		Detail: "the system was restored from " + what[0] + ". The root it replaced is kept as " + what[1] + " for two weeks.",
		At:     now()})
	return syncSnapshotMenu()
}

// blsKernels lists the kernel versions that have a plain Fedora BLS entry:
// no rescue image, and none of the debug or Ryoku console twins.
func blsKernels(dir, mid string) []string {
	var out []string
	matches, _ := filepath.Glob(filepath.Join(dir, mid+"-*.conf"))
	for _, m := range matches {
		kver := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(m), mid+"-"), ".conf")
		if strings.HasPrefix(kver, "0-rescue") || strings.Contains(kver, "~") {
			continue
		}
		out = append(out, kver)
	}
	return out
}

func readText(path string) string {
	b, _ := os.ReadFile(path)
	return string(b)
}

func writeFileAtomic(path, content string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".new"
	if err := os.WriteFile(tmp, []byte(content), mode); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
