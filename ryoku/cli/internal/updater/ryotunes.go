package updater

import (
	"context"
	"time"

	"ryoku-cli/internal/ryotunesrelease"
	"ryoku-cli/internal/sys"

	i18n "ryoku-i18n"
)

// upgradeRyotunes moves Ryotunes to its latest published GitHub release as part
// of `ryoku update`. Ryotunes is released on its own cadence as a prebuilt Arch
// package (ryoku-dev/ryotunes releases), not through the [ryoku] pacman repo, so
// it updates on its own channel: internal/ryotunesrelease re-reads the release
// fresh, verifies the download by sha256 and by its own pacman metadata (name,
// version, x86_64), installs it with pacman -U, and only ever moves the version
// forward. On pacman systems it runs outside the [ryoku] package set, so a box
// with no Ryoku changes still picks up a new Ryotunes. RPM systems leave
// Ryotunes to their package manager instead.
//
// Best-effort, like the AUR and Flatpak lanes: a failure is reported and never
// fails the whole update, and an offline or failed check is never rendered as
// "up to date". Not installed -> nothing to do: Upgrade does no network and
// installs nothing, so a Ryotunes the user removed is not resurrected.
var (
	checkRyotunesRelease   = ryotunesrelease.Check
	upgradeRyotunesRelease = ryotunesrelease.Upgrade
)

func upgradeRyotunes() {
	if sys.RPMManager() != "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	st, err := upgradeRyotunesRelease(ctx)
	switch {
	case err != nil:
		progress.logf(i18n.T("Could not update Ryotunes: %v"), err)
	case st.Installed == "":
		// Ryotunes is not installed; nothing to update (a removal stays removed).
	case st.Updated:
		progress.logf(i18n.T("Ryotunes updated to %s"), st.Latest)
	default:
		progress.logf(i18n.T("Ryotunes is current (%s)"), st.Installed)
	}
}

// addRyotunesUpdate folds a pending Ryotunes release into the Ryoku-lane report
// on pacman systems. RPM systems report Ryotunes through the distribution lane
// instead, so they return before consulting the release client. Bounded and
// read-only: Check never installs and does no network when Ryotunes is absent. An error
// or an up-to-date box adds nothing and never clears the rest of the report, so
// an offline poll is never rendered as a spurious update nor as a false
// "current".
func addRyotunesUpdate(r *statusReport) {
	if sys.RPMManager() != "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	st, err := checkRyotunesRelease(ctx)
	if err != nil || !st.Available {
		return
	}
	r.Updates = append(r.Updates, updateItem{Name: "ryotunes", Old: st.Installed, New: st.Latest})
	r.Behind++
	r.Available = true
}
