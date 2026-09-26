package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// bootOKDir is where a session records that the desktop came up in this boot:
// a sticky, world-writable directory the ryoku package ships through
// tmpfiles, so an unprivileged session can write its own marker and the
// root-side boot guard (ryoku boot-guard, ryoku-boot-guard.service) can read
// it on the next boot. One file per uid; the content is the boot id.
const bootOKDir = "/var/lib/ryoku/boot"

// bootOKSettle is how long the shell must stay up before the boot counts as
// good: past the supervisor's crash window with margin, short enough that a
// login followed by a quick logout still records it.
const bootOKSettle = 45 * time.Second

// recordBootOK waits for the shell to prove itself and writes the marker for
// this boot. Best effort: a missing directory (an older package) or an
// unwritable one leaves the guard without a signal, which it treats as an
// unproven boot, never as a failure of its own.
func recordBootOK(exited <-chan struct{}) {
	select {
	case <-exited:
		return // died inside the window; not a good boot
	case <-time.After(bootOKSettle):
	}
	markGrubBootSuccess()
	id, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		return
	}
	if st, err := os.Stat(bootOKDir); err != nil || !st.IsDir() {
		return
	}
	path := filepath.Join(bootOKDir, fmt.Sprintf("ok-%d", os.Getuid()))
	_ = os.WriteFile(path, []byte(strings.TrimSpace(string(id))+"\n"), 0o644)
}

// markGrubBootSuccess sets GRUB's boot_success flag. Fedora's GRUB hides its
// menu unless the previous boot never set the flag. Stock Fedora sets it from
// grub-boot-success.timer two minutes into any login, which ryoku-desktop
// turns off, so here it means the desktop came up: a boot that never got that
// far shows the menu, with its Ryoku console entries, the next time.
// grub2-set-bootflag is setuid root on Fedora.
func markGrubBootSuccess() {
	bin := "/usr/sbin/grub2-set-bootflag"
	if _, err := os.Stat(bin); err != nil {
		return
	}
	_ = exec.Command(bin, "boot_success").Run()
}
