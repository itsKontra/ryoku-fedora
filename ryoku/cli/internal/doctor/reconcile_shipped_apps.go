package doctor

import (
	"context"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"

	"ryoku-cli/internal/ryokumanifest"
	"ryoku-cli/internal/sys"

	i18n "ryoku-i18n"
)

// shippedApp is one deliver-once package: installed once, then left alone if
// the user removes it. Membership rule: a standalone application whose absence
// costs only itself. Tools the shell calls by name (grim, playerctl, matugen,
// cava, mpv for the launcher's radio, the pill's OCR/capture backends) stay hard
// depends, because losing them breaks a Ryoku surface the user never touched.
// Ryotunes has its own official-release install/reconciliation path.
type shippedApp struct {
	pkg  string
	what string
}

// shippedApps is the deliver-once table. It lives in the manifest package so the
// release's control manifest and this reconciler read one list, never two.
func shippedApps() []shippedApp {
	apps := ryokumanifest.Apps()
	out := make([]shippedApp, 0, len(apps))
	for _, a := range apps {
		out = append(out, shippedApp{pkg: a.Pkg, what: a.What})
	}
	return out
}

type appPlan struct {
	install  []string // never seen here and absent: deliver once
	removed  []string // ledgered and gone: the user's call, honoured
	adopt    []string // present but unrecorded: ledger it
	explicit []string // present and installed-as-dependency: re-mark explicit
}

// planShippedApps is the three-way rule, pure so it is tested without a package
// manager.
func planShippedApps(apps []shippedApp, installed, asDep, seen map[string]bool) appPlan {
	var p appPlan
	for _, a := range apps {
		switch {
		case installed[a.pkg]:
			if !seen[a.pkg] {
				p.adopt = append(p.adopt, a.pkg)
			}
			if asDep[a.pkg] {
				p.explicit = append(p.explicit, a.pkg)
			}
		case seen[a.pkg]:
			p.removed = append(p.removed, a.pkg)
		default:
			p.install = append(p.install, a.pkg)
		}
	}
	return p
}

// Seams: the live box's answers, replaced in tests.
var (
	// appPackager names the package manager the lane drives: pacman on Arch, dnf
	// on Fedora, "" on a box with neither.
	appPackager = func() string {
		switch {
		case sys.Has("pacman"):
			return "pacman"
		case sys.Has("dnf"):
			return "dnf"
		}
		return ""
	}
	appInstalled      = func(pkg string) bool { return sys.PkgInstalled(pkg) }
	appInstalledAsDep = func(pkg string) bool {
		if appPackager() == "dnf" {
			out, err := exec.Command("dnf", "repoquery", "-q", "--installed", "--qf", "%{reason}", pkg).Output()
			return err == nil && strings.Contains(string(out), "Dependency")
		}
		// `pacman -Qdq <pkg>` succeeds only for a package installed as a dependency.
		return exec.Command("pacman", "-Qdq", pkg).Run() == nil
	}
	// Fedora has no RPM yet for some apps (installation/fedora/README.md lists the
	// porting gaps). Those are skipped rather than warned about on every run, and
	// delivered once a repository starts carrying them. A failed query answers
	// "all available" so an offline box still gets the install attempt and its fix.
	appsAvailable = func(pkgs []string) map[string]bool {
		all := map[string]bool{}
		for _, p := range pkgs {
			all[p] = true
		}
		if appPackager() != "dnf" {
			return all
		}
		args := append([]string{"repoquery", "-q", "--available", "--qf", "%{name}\\n"}, pkgs...)
		out, err := exec.Command("dnf", args...).Output()
		if err != nil {
			return all
		}
		found := map[string]bool{}
		for _, name := range strings.Fields(string(out)) {
			found[name] = true
		}
		return found
	}
	// One transaction for the whole missing set, bounded, and best-effort: a box
	// with no network must not fail `ryoku update` over an app.
	installShippedApps = func(pkgs []string) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		args := append([]string{"pacman", "-S", "--needed", "--noconfirm"}, pkgs...)
		if appPackager() == "dnf" {
			args = append([]string{"dnf", "install", "-y"}, pkgs...)
		}
		_ = exec.CommandContext(ctx, "sudo", args...).Run()
	}
	markAppsExplicit = func(pkgs []string) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		args := append([]string{"pacman", "-D", "--asexplicit", "--quiet"}, pkgs...)
		if appPackager() == "dnf" {
			args = append([]string{"dnf", "-y", "mark", "user"}, pkgs...)
		}
		_ = exec.CommandContext(ctx, "sudo", args...).Run()
	}
)

// installFix is the command a user runs to land pkgs by hand.
func installFix(pkgs []string) string {
	if appPackager() == "dnf" {
		return "sudo dnf install " + strings.Join(pkgs, " ")
	}
	return "sudo pacman -Sy && sudo pacman -S " + strings.Join(pkgs, " ")
}

func reconcileShippedApps(checkOnly bool) recResult {
	if appPackager() == "" {
		return okRes(i18n.T("no pacman or dnf here; shipped apps are the installer's business"))
	}
	apps := shippedApps()
	installed, asDep := map[string]bool{}, map[string]bool{}
	for _, a := range apps {
		if appInstalled(a.pkg) {
			installed[a.pkg] = true
			asDep[a.pkg] = appInstalledAsDep(a.pkg)
		}
	}
	plan := planShippedApps(apps, installed, asDep, provisioned())
	var unavailable []string
	if len(plan.install) > 0 {
		avail := appsAvailable(plan.install)
		var ready []string
		for _, pkg := range plan.install {
			if avail[pkg] {
				ready = append(ready, pkg)
			} else {
				unavailable = append(unavailable, pkg)
			}
		}
		plan.install = ready
	}

	if len(plan.install) == 0 && len(plan.adopt) == 0 && len(plan.explicit) == 0 {
		var notes []string
		if len(plan.removed) > 0 {
			notes = append(notes, fmt.Sprintf(i18n.T("%s stay removed (you deleted them; Ryoku does not put them back)"),
				strings.Join(plan.removed, ", ")))
		}
		if len(unavailable) > 0 {
			notes = append(notes, fmt.Sprintf(i18n.T("%s not packaged for this system yet"),
				strings.Join(unavailable, ", ")))
		}
		if len(notes) > 0 {
			return noteRes("%s", strings.Join(notes, "; "))
		}
		return okRes(i18n.T("every shipped app is present and owned by you"))
	}
	if checkOnly {
		var parts []string
		if len(plan.install) > 0 {
			parts = append(parts, i18n.Tf("would install %s", strings.Join(plan.install, ", ")))
		}
		if len(plan.explicit) > 0 || len(plan.adopt) > 0 {
			parts = append(parts, fmt.Sprintf(i18n.T("would take ownership of %d present app(s)"),
				len(union(plan.adopt, plan.explicit))))
		}
		return wouldRes("%s", strings.Join(parts, "; ")).
			withFix(i18n.T("run `ryoku doctor` (or `ryoku update`) to apply"))
	}

	// Ownership first: it cannot fail the run, and it protects what is already
	// here even if the install half finds no mirror.
	if len(plan.explicit) > 0 {
		markAppsExplicit(plan.explicit)
	}
	for _, pkg := range plan.adopt {
		recordProvisioned(pkg)
	}

	var landed, missed []string
	if len(plan.install) > 0 {
		installShippedApps(plan.install)
		for _, pkg := range plan.install {
			if appInstalled(pkg) {
				recordProvisioned(pkg)
				landed = append(landed, pkg)
			} else {
				missed = append(missed, pkg)
			}
		}
		if len(landed) > 0 {
			markAppsExplicit(landed)
		}
	}

	switch {
	case len(missed) > 0 && len(landed) > 0:
		return warnRes(i18n.T("installed %s; %s did not land"), strings.Join(landed, ", "), strings.Join(missed, ", ")).
			withFix("%s", installFix(missed))
	case len(missed) > 0:
		return warnRes(i18n.T("%s could not be installed"), strings.Join(missed, ", ")).
			withFix("%s", installFix(missed))
	case len(landed) > 0:
		return fixedRes(i18n.T("installed %s (delete any of them and Ryoku will not reinstall it)"),
			strings.Join(landed, ", "))
	}
	return fixedRes(i18n.T("took ownership of %d shipped app(s) so an orphan sweep cannot remove them"),
		len(union(plan.adopt, plan.explicit)))
}

func union(a, b []string) []string {
	seen := map[string]bool{}
	for _, s := range append(append([]string{}, a...), b...) {
		seen[s] = true
	}
	out := make([]string, 0, len(seen))
	for s := range seen {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}
