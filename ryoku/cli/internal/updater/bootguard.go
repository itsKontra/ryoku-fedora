package updater

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"ryoku-cli/internal/sys"
	i18n "ryoku-i18n"
)

// The boot guard falls back from a packaged update whose next boots never
// bring the desktop up. The pieces:
//
//   - stage2 arms it: pendingFile records the release the box ran before, the
//     one it moved to, the pre-update snapshot, and the boot the update ran in.
//   - the shell daemon records each boot that reached a Ryoku session: it
//     writes bootOKDir/session-<uid> with the boot id when it starts the
//     shell, and bootOKDir/ok-<uid> once the shell has stayed up past its
//     crash window.
//   - ryoku-boot-guard.service runs `ryoku boot-guard` early in every boot,
//     as root. With a marker present and no ok file from a boot after the
//     update, it counts the previous boot if a Ryoku session started in it;
//     on the third such boot it sends the next boot, once, to the pre-update
//     snapshot's read-only entry and leaves a notice the doctor surfaces. A
//     boot no one logged into Ryoku from (a greeter left idle, another
//     desktop, a headless boot) proves nothing either way and is not counted.
//
// Nothing here needs the user session, so it works when the session is what
// broke.
const (
	pendingFile = "/var/lib/ryoku/update-pending.json"
	noticeFile  = "/var/lib/ryoku/boot/notice.json"
)

// bootOKDir holds the session's boot markers; a var for tests.
var bootOKDir = "/var/lib/ryoku/boot"

type pendingUpdate struct {
	From      string   `json:"from"`
	To        string   `json:"to"`
	Snapshot  string   `json:"snapshot,omitempty"`
	ArmedBoot string   `json:"armedBoot"`
	Boots     int      `json:"boots"`
	Counted   []string `json:"counted,omitempty"` // the boot ids already counted
	At        string   `json:"at"`
}

// bootNotice is what the doctor shows after the guard acted.
type bootNotice struct {
	Action   string `json:"action"` // snapshot-default, revert-failed, snapshot-restored
	From     string `json:"from"`
	To       string `json:"to"`
	Snapshot string `json:"snapshot,omitempty"`
	Detail   string `json:"detail,omitempty"`
	At       string `json:"at"`
}

func bootID() string {
	b, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// armBootGuard is called by stage2 on a packaged box once the packages are
// in. RYOKU_UPDATE_FROM carries the release the first stage read before dnf
// ran; the marker is written whenever the release changes, a rolling COPR
// build included, since only boots that reached a Ryoku session count.
func armBootGuard(snapshot string) {
	if sys.ResolveRepo() != "" {
		return
	}
	from := strings.TrimSpace(os.Getenv("RYOKU_UPDATE_FROM"))
	to := sys.ReadRelease().Release
	if from == "" || to == "" || from == to {
		return
	}
	p := pendingUpdate{From: from, To: to, Snapshot: snapshot, ArmedBoot: bootID(), At: time.Now().UTC().Format(time.RFC3339)}
	b, _ := json.MarshalIndent(p, "", "  ")
	// earlier markers would read as boots after this update; clear them so
	// only a boot from here on counts.
	var markers []string
	for _, kind := range []string{"ok-*", "session-*"} {
		m, _ := filepath.Glob(filepath.Join(bootOKDir, kind))
		markers = append(markers, m...)
	}
	if len(markers) > 0 {
		_ = sys.Sudo(append([]string{"rm", "-f"}, markers...)...)
	}
	if err := sys.WriteRootFile(pendingFile, string(b)+"\n", "0644"); err != nil {
		fmt.Fprintf(os.Stderr, i18n.T("note: boot guard not armed: %v\n"), err)
		return
	}
	progress.logf(i18n.T("Boot guard armed: %s -> %s"), from, to)
}

// BootGuard is `ryoku boot-guard`, run as root by ryoku-boot-guard.service.
func BootGuard(args []string) error {
	if os.Geteuid() != 0 {
		return fmt.Errorf(i18n.T("ryoku boot-guard runs as root (ryoku-boot-guard.service)"))
	}
	if len(args) > 0 && args[0] == "--disarm" {
		return disarmBootGuard(i18n.T("disarmed by hand"))
	}
	if len(args) > 0 && args[0] == "--console" {
		return consoleGuard()
	}
	raw, err := os.ReadFile(pendingFile)
	if err != nil {
		return nil // nothing pending
	}
	var p pendingUpdate
	if err := json.Unmarshal(raw, &p); err != nil {
		_ = os.Remove(pendingFile)
		return nil
	}
	if provenAfter(p.ArmedBoot) {
		fmt.Printf(i18n.T("boot guard: %s came up after the update; disarmed\n"), p.To)
		return disarmBootGuard("")
	}
	failed := sessionBoots(p, bootID())
	if len(failed) == 0 {
		fmt.Printf(i18n.T("boot guard: no Ryoku session since the last boot; %s -> %s still pending\n"), p.From, p.To)
		return nil
	}
	p.Boots++
	p.Counted = append(p.Counted, failed...)
	b, _ := json.MarshalIndent(p, "", "  ")
	_ = os.WriteFile(pendingFile, append(b, '\n'), 0o644)
	fmt.Printf(i18n.T("boot guard: boot %d after %s -> %s without the desktop coming up\n"), p.Boots, p.From, p.To)
	if p.Boots < 3 {
		return nil
	}
	return grubBootSnapshot(p)
}

// provenAfter reports whether any session recorded a good boot other than
// the one the update ran in.
func provenAfter(armedBoot string) bool {
	for _, id := range markerBoots("ok-*") {
		if id != armedBoot {
			return true
		}
	}
	return false
}

// sessionBoots is the boots a Ryoku session started in since the update and
// that are not counted yet: not the boot the update ran in, not this one (the
// guard runs before any login, so a marker naming it is a rerun by hand).
func sessionBoots(p pendingUpdate, current string) []string {
	var out []string
	for _, id := range markerBoots("session-*") {
		if id == p.ArmedBoot || id == current || slices.Contains(p.Counted, id) || slices.Contains(out, id) {
			continue
		}
		out = append(out, id)
	}
	return out
}

// markerBoots reads the boot ids the markers matching pattern record.
func markerBoots(pattern string) []string {
	matches, _ := filepath.Glob(filepath.Join(bootOKDir, pattern))
	var ids []string
	for _, m := range matches {
		b, err := os.ReadFile(m)
		if err != nil {
			continue
		}
		if id := strings.TrimSpace(string(b)); id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

func disarmBootGuard(why string) error {
	_ = os.Remove(pendingFile)
	if why != "" {
		fmt.Println("boot guard:", why)
	}
	return nil
}

// grubBootSnapshot sends the next boot, once, to the pre-update snapshot's
// read-only entry. It never restores unattended: the user sees the system from
// before the update and decides with `ryoku rollback <id>`, and a boot after
// that one without a decision is back on the updated system.
func grubBootSnapshot(p pendingUpdate) error {
	fail := func(detail string) error {
		return writeNotice(bootNotice{Action: "revert-failed", From: p.From, To: p.To, Snapshot: p.Snapshot, Detail: detail, At: now()})
	}
	if p.Snapshot == "" {
		return fail("no pre-update snapshot to boot; pick one under Ryoku snapshots in the boot menu")
	}
	if !strings.Contains(readText(snapshotMenuCfg()), " --id ryoku-snapshot-"+p.Snapshot+"-preview ") {
		return fail("snapshot " + p.Snapshot + " is not in the boot menu; pick another under Ryoku snapshots")
	}
	if err := sys.Run("grub2-reboot", snapshotEntryID(p.Snapshot, "preview")); err != nil {
		return fail("could not set the next boot to snapshot " + p.Snapshot + ": " + err.Error())
	}
	_ = os.Remove(pendingFile)
	return writeNotice(bootNotice{Action: "snapshot-default", From: p.From, To: p.To, Snapshot: p.Snapshot,
		Detail: "three boots failed after the update, so the next boot opens pre-update snapshot " + p.Snapshot + " read-only. If it works, `ryoku rollback " + p.Snapshot + "` restores it for good.",
		At:     now()})
}

func writeNotice(n bootNotice) error {
	b, _ := json.MarshalIndent(n, "", "  ")
	_ = os.MkdirAll(bootOKDir, 0o1777)
	fmt.Println("boot guard:", n.Action, n.Detail)
	return os.WriteFile(noticeFile, append(b, '\n'), 0o644)
}

func now() string { return time.Now().UTC().Format(time.RFC3339) }

// BootNotice reads the guard's last notice, or nil.
func BootNotice() *bootNotice {
	b, err := os.ReadFile(noticeFile)
	if err != nil {
		return nil
	}
	var n bootNotice
	if json.Unmarshal(b, &n) != nil {
		return nil
	}
	return &n
}
