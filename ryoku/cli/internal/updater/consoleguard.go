package updater

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

// The console guard covers the failure sddm's OnFailure cannot see: the login
// screen dies while sddm itself stays up. sddm does not restart a greeter that
// exits; it leaves a black screen until the next boot. ryoku-console-guard.service
// runs `ryoku boot-guard --console` before sddm on every graphical boot and
// reads the previous boot's sddm journal. When that boot's last login screen
// closed outside a stop of sddm and nothing (a new greeter or a login)
// replaced it, the guard holds sddm back for this boot (the sddm drop-in's
// ConditionPathExists) and starts ryoku-console-fallback.service, a text login
// with a recovery banner on tty1. Only the previous boot counts, so the boot
// after a console boot tries the desktop again.
const consoleBootFlag = "/run/ryoku/console-boot"

// sddmEvents selects what greeterDied reads from sddm's journal: systemd's
// start/stop of the unit, and the PAM sessions sddm-helper opens and closes.
const sddmEvents = `^(Started|Stopping) sddm\.service|^pam_unix\(sddm(-greeter)?:session\): session (opened|closed)`

var (
	sddmStarted   = regexp.MustCompile(`^Started sddm\.service`)
	sddmStopping  = regexp.MustCompile(`^Stopping sddm\.service`)
	greeterOpened = regexp.MustCompile(`^pam_unix\(sddm-greeter:session\): session opened`)
	greeterClosed = regexp.MustCompile(`^pam_unix\(sddm-greeter:session\): session closed`)
	sessionOpened = regexp.MustCompile(`^pam_unix\(sddm:session\): session opened`)
)

func consoleGuard() error {
	out, err := exec.Command("journalctl", "-b", "-1", "-u", "sddm.service", "-q",
		"-o", "json", "--output-fields=MESSAGE", "-g", sddmEvents).Output()
	if err != nil || !greeterDied(out) {
		return nil // no previous boot in the journal, or its login screen was fine
	}
	fmt.Println("boot guard: the login screen died on the last boot and nothing replaced it; starting a console login")
	if err := os.MkdirAll("/run/ryoku", 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(consoleBootFlag, nil, 0o644); err != nil {
		return err
	}
	return exec.Command("systemctl", "start", "--no-block", "ryoku-console-fallback.service").Run()
}

// greeterDied reads journalctl's JSON lines for one boot, in order, and reports
// whether the last greeter session closed while sddm was running and was left
// that way: no new greeter and no login after it.
func greeterDied(journal []byte) bool {
	stopping, died := false, false
	sc := bufio.NewScanner(bytes.NewReader(journal))
	for sc.Scan() {
		var e struct {
			Message string `json:"MESSAGE"`
		}
		if json.Unmarshal(sc.Bytes(), &e) != nil {
			continue
		}
		msg := strings.TrimSpace(e.Message)
		switch {
		case sddmStarted.MatchString(msg):
			stopping = false
		case sddmStopping.MatchString(msg):
			stopping = true
		case greeterOpened.MatchString(msg), sessionOpened.MatchString(msg):
			died = false
		case greeterClosed.MatchString(msg):
			died = !stopping
		}
	}
	return died
}
