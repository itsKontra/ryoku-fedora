package doctor

import (
	"strings"

	"ryoku-cli/internal/sys"

	i18n "ryoku-i18n"
)

// ---- reconciler: Secure Boot signed NVIDIA driver ----------------------------

// reconcileNvidiaSigned keeps a Fedora box on the signed ryoku-nvidia driver
// working under Secure Boot. It never installs a driver by itself, since the
// host driver stays the user's choice: it points a Turing+ GPU left on nouveau
// at `ryoku-nvidia install`, and re-queues a MOK enrollment the user skipped,
// without which the signed modules cannot load. `ryoku-nvidia status` is the
// single source of the hardware, driver, and key facts.

var (
	nvidiaSignedStatus = func() string {
		out, _ := sys.RunOut("ryoku-nvidia", "status")
		return out
	}
	enrollNvidiaKey  = func() error { return sys.Sudo("ryoku-nvidia", "enroll") }
	nvidiaSignedHost = func() bool { return sys.RPMManager() != "" && sys.Has("ryoku-nvidia") }
)

func parseNvidiaStatus(out string) map[string]string {
	facts := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		if k, v, ok := strings.Cut(strings.TrimSpace(line), "="); ok {
			facts[k] = v
		}
	}
	return facts
}

func reconcileNvidiaSigned(checkOnly bool) recResult {
	if !nvidiaSignedHost() {
		return okRes(i18n.T("no signed NVIDIA driver helper on this system"))
	}
	facts := parseNvidiaStatus(nvidiaSignedStatus())
	if facts["gpu"] != "turing" {
		return okRes(i18n.T("no NVIDIA GPU that the signed open driver supports"))
	}
	switch facts["driver"] {
	case "host":
		return okRes(i18n.T("keeping the host NVIDIA driver"))
	case "none":
		return noteRes(i18n.T("this NVIDIA GPU runs on nouveau; the Secure Boot signed driver is available")).
			withFix("sudo ryoku-nvidia install")
	}
	if facts["secureboot"] != "on" {
		return okRes(i18n.T("signed NVIDIA driver installed; Secure Boot is off, so no key enrollment is needed"))
	}
	switch facts["key"] {
	case "enrolled":
		return okRes(i18n.T("signed NVIDIA driver installed and the Ryoku module key is enrolled"))
	case "pending":
		return warnRes(i18n.T("the Ryoku module key awaits approval: reboot, choose Enroll MOK, and type the password ryoku"))
	}
	if checkOnly {
		return wouldRes(i18n.T("the Ryoku module key is not enrolled, so the NVIDIA driver cannot load under Secure Boot")).
			withFix(i18n.T("ryoku doctor  (queues the key for MokManager)"))
	}
	if err := enrollNvidiaKey(); err != nil {
		return failRes(i18n.T("could not queue the Ryoku module key: %v"), err).
			withFix("sudo ryoku-nvidia enroll")
	}
	return fixedRes(i18n.T("queued the Ryoku module key: reboot, choose Enroll MOK, and type the password ryoku"))
}
