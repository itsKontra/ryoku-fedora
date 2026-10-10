package doctor

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"ryoku-cli/internal/sys"

	i18n "ryoku-i18n"
)

// ryotunesSocketUnit provides native daemon socket activation. The launcher can
// start it on demand; enabling it also makes cold session activation available.
const ryotunesSocketUnit = "ryotunesd.socket"

// Ryotunes is part of the Ryoku desktop but ships from its own COPR project
// (itskontra/ryotunes), so dnf moves it with the rest of the system. Two things
// keep a box from opening the packaged native client: a wrapper or a locally
// built copy in ~/.local/bin that shadows /usr/bin/ryotunes on PATH (a dev
// deploy laid both before the package existed), and a managed box -- a dev
// checkout (ResolveRepo) or one with the packaged ryoku-desktop -- simply
// missing the package, which the reconcile installs with dnf.
func reconcileRyotunes(checkOnly bool) recResult {
	var problems, fixes []string
	// Deliver-once like the shipped apps: while the package is here, keep it in
	// the provisioned ledger, so a later deletion reads as the user's choice
	// rather than a gap to heal. recordProvisioned is a no-op when recorded,
	// and a check-only run never writes.
	if !checkOnly && sys.PkgInstalled("ryotunes") {
		recordProvisioned("ryotunes")
	}

	bin := filepath.Join(sys.Home(), ".local", "bin", "ryotunes")
	stale := staleUserRyotunes(bin)
	if stale != "" {
		problems = append(problems, i18n.Tf("%s in ~/.local/bin shadows the packaged app", stale))
		fixes = append(fixes, "rm -f ~/.local/bin/ryotunes ~/.local/share/applications/ryotunes.desktop")
	}
	// A box that runs the Ryoku desktop is expected to have Ryotunes: a dev
	// checkout (all Ryoku managed by `ryoku deploy`, so ryoku-desktop is not an
	// installed package) or a packaged install (ryoku-desktop present). Either
	// way, if the app is absent the reconcile installs it --
	// unless the user deleted it: ryotunes is a deliver-once app like the rest
	// of the shipped set, so the provisioned ledger is honoured here too.
	managedDesktop := sys.ResolveRepo() != "" || sys.PkgInstalled("ryoku-desktop")
	desktopMissingRyotunes := managedDesktop && !sys.PkgInstalled("ryotunes") && !removedByUser("ryotunes")
	if desktopMissingRyotunes {
		problems = append(problems, i18n.T("the ryotunes package is not installed"))
		fixes = append(fixes, "sudo dnf install ryotunes")
	}
	socketMissing := sys.PkgInstalled("ryotunes") && !ryotunesSocketEnabled()
	if socketMissing {
		problems = append(problems, i18n.T("the ryotunesd socket is not enabled for session activation"))
		fixes = append(fixes, "systemctl --user enable --now ryotunesd.socket")
	}
	if len(problems) == 0 {
		return okRes(i18n.T("ryotunes is the packaged app"))
	}
	if checkOnly {
		return wouldRes("%s", strings.Join(problems, "; ")).withFix(strings.Join(fixes, " && "))
	}
	if stale != "" {
		appshare := sys.Xdg("XDG_DATA_HOME", ".local/share")
		for _, p := range []string{
			bin,
			filepath.Join(appshare, "applications", "ryotunes.desktop"),
			filepath.Join(appshare, "ryoku", "ryotunes.commit"),
			filepath.Join(appshare, "icons", "hicolor", "scalable", "apps", "ryotunes.svg"),
		} {
			_ = os.Remove(p)
		}
		icons, _ := filepath.Glob(filepath.Join(appshare, "icons", "hicolor", "*", "apps", "ryotunes.png"))
		for _, p := range icons {
			_ = os.Remove(p)
		}
	}
	if desktopMissingRyotunes {
		if err := sys.Sudo("dnf", "-y", "install", "ryotunes"); err != nil {
			return failRes(i18n.T("could not install ryotunes: %v"), err).withFix("sudo dnf install ryotunes")
		}
	}
	if socketMissing || (desktopMissingRyotunes && sys.PkgInstalled("ryotunes") && !ryotunesSocketEnabled()) {
		// daemon-reload so a unit the package just delivered is known, then
		// enable --now: the socket binds in this session without a relogin.
		_ = exec.Command("systemctl", "--user", "daemon-reload").Run()
		if err := exec.Command("systemctl", "--user", "enable", "--now", ryotunesSocketUnit).Run(); err != nil {
			return failRes(i18n.T("could not enable %s: %v"), ryotunesSocketUnit, err).
				withFix("systemctl --user enable --now ryotunesd.socket")
		}
	}
	return fixedRes(i18n.T("ryotunes opens the packaged app (%s)"), strings.Join(problems, "; "))
}

func ryotunesSocketEnabled() bool {
	out, _ := exec.Command("systemctl", "--user", "is-enabled", ryotunesSocketUnit).Output()
	return strings.TrimSpace(string(out)) == "enabled"
}

// staleUserRyotunes names what ~/.local/bin/ryotunes is when it is not the
// user's own program: the Chromium wrapper (a script that opens
// music.youtube.com) or a build the dev deploy recorded a commit for.
func staleUserRyotunes(bin string) string {
	st, err := os.Stat(bin)
	if err != nil || st.IsDir() {
		return ""
	}
	head := make([]byte, 4096)
	if f, err := os.Open(bin); err == nil {
		n, _ := f.Read(head)
		f.Close()
		head = head[:n]
	}
	if strings.HasPrefix(string(head), "#!") && strings.Contains(string(head), "music.youtube.com") {
		return "the Chromium YouTube Music wrapper"
	}
	if sys.Exists(filepath.Join(sys.Xdg("XDG_DATA_HOME", ".local/share"), "ryoku", "ryotunes.commit")) {
		return "a locally built ryotunes"
	}
	return ""
}
