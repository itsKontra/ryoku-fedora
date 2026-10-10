package doctor

import (
	"os"
	"strings"

	"ryoku-cli/internal/sys"

	i18n "ryoku-i18n"
)

// ---- reconciler: fingerprint unlock module -----------------------------------
//
// The in-session lock (qylock) and the SDDM greeter authenticate through a PAM
// stack that loads pam_fprintd_grosshack.so -- a fork of pam_fprintd that starts
// fprintd-verify at the beginning of the conversation, so the sensor scans while
// the password field is live and the first success (touch OR type) wins. The
// lock's PAM service (ryoku/lockscreen/.../assets/pam/ryoku-lock) names that
// module unconditionally, and the Hub's Lockscreen page only offers the sddm/sudo
// fingerprint toggles when it is present.
//
// The module has no Fedora package yet (installation/fedora/README.md), so a
// box with a reader has the whole fingerprint-unlock feature wired up with the
// module missing: fprintd's own enroll/verify (Ryoku Settings) still works, but
// the lock and greeter silently do nothing on a touch. Say so on a box with a
// reader; stay silent on one without.

// fingerprintModulePath is where the grosshack PAM module lands, the exact path
// the lock's PAM stack and the Hub probe. A var so a test can point it elsewhere.
var fingerprintModulePath = "/usr/lib/security/pam_fprintd_grosshack.so"

// fingerprintReaderPresent reports whether fprintd sees a fingerprint device. A
// var seam so planFingerprintModule stays a pure function testable without
// hardware. fprintd-list enumerates the reader(s) for a user and prints each
// device's object path; no reader (or no fprintd) yields neither, i.e. false.
var fingerprintReaderPresent = func() bool {
	if !sys.Has("fprintd-list") {
		return false
	}
	user := os.Getenv("USER")
	if user == "" {
		return false
	}
	out, err := sys.RunOut("fprintd-list", user)
	if err != nil {
		return false
	}
	return strings.Contains(out, "/net/reactivated/Fprint/Device")
}

// planFingerprintModule decides the result from the facts it depends on: is
// there a reader, and is the module already installed. pure, so every branch is
// unit-testable without fprintd.
func planFingerprintModule(readerPresent, moduleInstalled bool) recResult {
	switch {
	case !readerPresent:
		return okRes(i18n.T("no fingerprint reader; the unlock module is not needed"))
	case moduleInstalled:
		return okRes(i18n.T("fingerprint unlock module (pam-fprint-grosshack) present"))
	default:
		return noteRes(i18n.T("pam-fprint-grosshack has no Fedora package yet; touch-to-unlock at the lock and login screens stays off"))
	}
}

func reconcileFingerprintModule(_ bool) recResult {
	return planFingerprintModule(fingerprintReaderPresent(), sys.Exists(fingerprintModulePath))
}
