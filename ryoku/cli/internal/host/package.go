package host

import "fmt"

// Package runs a package verb against dnf and rpm. Queries go through the
// Packages primitives; transactions run dnf non-interactively with the caller's
// stdio, so they need the caller to have escalated (sudo or pkexec).
func (a *App) Package(args []string) int {
	if len(args) == 0 {
		return ExitUsage
	}
	verb, rest := args[0], args[1:]
	dnf := a.cfg.Packages.Manager()
	if dnf == "" {
		return a.failf("no supported package manager found")
	}
	switch verb {
	case "installed":
		if len(rest) == 0 {
			return ExitUsage
		}
		for _, name := range rest {
			if !a.cfg.Packages.Installed(name) {
				return ExitFalse
			}
		}
		return ExitOK
	case "available":
		if len(rest) == 0 {
			return ExitUsage
		}
		for _, name := range rest {
			if !a.cfg.Packages.Available(name) {
				return ExitFalse
			}
		}
		return ExitOK
	case "version":
		if len(rest) != 1 {
			return ExitUsage
		}
		version := a.cfg.Packages.Version(rest[0])
		if version == "" {
			return ExitFalse
		}
		fmt.Fprintln(a.cfg.Stdout, version)
		return ExitOK
	case "owner":
		if len(rest) != 1 {
			return ExitUsage
		}
		owner := a.cfg.Packages.Owner(rest[0])
		if owner == "" {
			return ExitFalse
		}
		fmt.Fprintln(a.cfg.Stdout, owner)
		return ExitOK
	case "install":
		action := "install"
		if len(rest) > 0 && rest[0] == "--upgrade" {
			action, rest = "upgrade", rest[1:]
		}
		if len(rest) == 0 {
			return ExitUsage
		}
		return commandExit(a.run(dnf, append([]string{"-y", action, "--"}, rest...)...))
	case "install-file":
		if len(rest) == 0 {
			return ExitUsage
		}
		return commandExit(a.run(dnf, append([]string{"install", "-y"}, rest...)...))
	case "remove":
		if len(rest) == 0 {
			return ExitUsage
		}
		var installed []string
		for _, name := range rest {
			if a.cfg.Packages.Installed(name) {
				installed = append(installed, name)
			}
		}
		if len(installed) == 0 {
			return ExitOK
		}
		// Unattended, so it removes only what was named: the unused-dependency
		// sweep stays with an interactive remove the user reviews.
		argv := append([]string{"-y", "--setopt=clean_requirements_on_remove=False", "remove", "--"}, installed...)
		return commandExit(a.run(dnf, argv...))
	}
	return ExitUsage
}
