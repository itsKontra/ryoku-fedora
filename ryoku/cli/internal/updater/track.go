package updater

import (
	"fmt"
	"os"
	"os/exec"

	"ryoku-cli/internal/sys"
	i18n "ryoku-i18n"
)

// Track moves a packaged box onto the COPR channel. It rewrites the RyokuCOPR
// baseurl and runs an update, which moves the Ryoku set to what the channel
// serves. A box whose updates currently come from a source checkout is migrated
// onto packages first: the checkout is retired as the update source (the clone
// stays on disk but no longer drives updates). Building from a checkout is
// `ryoku track ... --source`.
func Track(channel string) error {
	if sys.ChannelServer(channel) == "" {
		return fmt.Errorf(i18n.T("Fedora uses one rolling COPR repository; use ryoku track copr. Stable/testing channels and tag rollback are unavailable"))
	}
	source := sys.SourceTracked()
	install := !sys.PkgInstalled("ryoku-desktop")
	// A pure source box with no ryoku-desktop and no repo to install it from
	// cannot be moved onto packages here.
	if install && sys.RyokuServer() == "" {
		return fmt.Errorf(i18n.T("no ryoku-desktop package and no %s repo to install it from; reinstall the Ryoku repository, then `ryoku track %s`"), sys.RPMRepoName, channel)
	}
	// A deliberate private mirror (a baseurl Ryoku does not publish) is never
	// silently overwritten, unless we are migrating a source box off its checkout.
	if !source && sys.PackagedChannel() == "" && sys.RyokuServer() != "" {
		return fmt.Errorf(i18n.T("the %s repo points at %s, a mirror Ryoku does not publish; edit %s by hand"), sys.RPMRepoName, sys.RyokuServer(), sys.RPMRepoFile)
	}

	migrated, err := switchToPackageChannel(channel)
	if err != nil {
		return err
	}
	if migrated {
		clearSessionChannelEnv()
		fmt.Println(i18n.T("==> Retired the source checkout as the update source; the clone stays on disk but no longer drives updates."))
	}
	fmt.Println(i18n.T("==> Now tracking the Ryoku COPR packages."))
	if install {
		fmt.Println(i18n.T("==> ryoku-desktop is not installed here; the channel switch installs it from the selected channel."))
	}
	fmt.Println(i18n.T("==> Updates now come from packages: `ryoku update` runs dnf."))
	return runChannelUpdate()
}

// switchToPackageChannel performs the filesystem side of a packaged track:
// migrate a source-tracked box off its checkout (retire tracking, keep the
// clone) and rewrite the RyokuCOPR baseurl to channel. It touches no package
// manager and no network, so it is unit-testable; the caller runs dnf after.
func switchToPackageChannel(channel string) (migrated bool, err error) {
	if err := sys.SetPackagedChannel(channel); err != nil {
		return false, err
	}
	if sys.SourceTracked() {
		if err := sys.RetireSourceTracking(); err != nil {
			return false, fmt.Errorf(i18n.T("could not retire the source checkout: %w"), err)
		}
		migrated = true
	}
	return migrated, nil
}

// runChannelUpdate is the dnf side of a track: an ordinary update against the
// rewritten repo. A var so a test exercises the switch without running dnf.
var runChannelUpdate = func() error { return Update(nil) }

// clearSessionChannelEnv drops RYOKU_CHANNEL from the running user manager and
// the D-Bus activation env (the reverse of bin/ryoku-track), so units and apps
// launched after a migration no longer carry the stale channel. Best-effort and
// a var so a test never touches the real session.
var clearSessionChannelEnv = func() {
	_ = exec.Command("systemctl", "--user", "unset-environment", "RYOKU_CHANNEL").Run()
	_ = exec.Command("dbus-update-activation-environment", "RYOKU_CHANNEL=").Run()
	os.Unsetenv("RYOKU_CHANNEL")
}
