package updater

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// The console guard covers the failure sddm's OnFailure cannot see: sddm stays
// up while the login screen (or a session that never proves itself) keeps
// restarting. ryoku-console-guard.service runs `ryoku boot-guard --console`
// before sddm on every graphical boot. When the previous boot opened the
// greeter greeterLoopStarts times or more and no session wrote a boot-ok
// marker for it, the guard holds sddm back for this boot (the sddm drop-in's
// ConditionPathExists) and starts ryoku-console-fallback.service, which puts
// a text login with a recovery banner on tty1. Only the previous boot counts,
// so the boot after a console boot tries the desktop again.
const (
	consoleBootFlag   = "/run/ryoku/console-boot"
	greeterLoopStarts = 3
	// each greeter start opens this PAM session in sddm's journal
	greeterOpened = `^pam_unix\(sddm-greeter:session\): session opened`
)

func consoleGuard() error {
	out, err := exec.Command("journalctl", "-b", "-1", "-u", "sddm.service", "-q",
		"-o", "json", "--output-fields=_BOOT_ID", "-g", greeterOpened).Output()
	if err != nil {
		return nil // no previous boot in the journal, or no greeter in it
	}
	boot, starts := greeterStarts(out)
	if starts < greeterLoopStarts || provenBoots()[boot] {
		return nil
	}
	fmt.Printf("boot guard: the login screen started %d times on the last boot and no session came up; starting a console login\n", starts)
	if err := os.MkdirAll("/run/ryoku", 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(consoleBootFlag, []byte(boot+"\n"), 0o644); err != nil {
		return err
	}
	return exec.Command("systemctl", "start", "--no-block", "ryoku-console-fallback.service").Run()
}

// greeterStarts reads journalctl's JSON lines for one boot and returns that
// boot's id (normalized to the dashed /proc form the boot-ok markers carry)
// and how many greeter sessions it opened.
func greeterStarts(journal []byte) (boot string, starts int) {
	sc := bufio.NewScanner(bytes.NewReader(journal))
	for sc.Scan() {
		var e struct {
			BootID string `json:"_BOOT_ID"`
		}
		if json.Unmarshal(sc.Bytes(), &e) != nil || e.BootID == "" {
			continue
		}
		boot = dashedBootID(e.BootID)
		starts++
	}
	return boot, starts
}

// dashedBootID turns the journal's 32-hex boot id into the 8-4-4-4-12 form
// /proc/sys/kernel/random/boot_id prints.
func dashedBootID(id string) string {
	id = strings.ToLower(strings.ReplaceAll(id, "-", ""))
	if len(id) != 32 {
		return id
	}
	return id[:8] + "-" + id[8:12] + "-" + id[12:16] + "-" + id[16:20] + "-" + id[20:]
}
