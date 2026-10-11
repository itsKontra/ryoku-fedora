package host

import (
	"fmt"
	"os"
)

// InitVerb prints the init system. Fedora boots systemd; a box where it is not
// running (a container, a chroot) has no init this seam can drive.
func (a *App) InitVerb(args []string) int {
	if len(args) != 0 {
		return ExitUsage
	}
	info, err := os.Stat(a.cfg.SystemdRuntimeDir)
	if err != nil || !info.IsDir() {
		fmt.Fprintln(a.cfg.Stderr, "ryoku-host: systemd is not running")
		return ExitAbsent
	}
	fmt.Fprintln(a.cfg.Stdout, "systemd")
	return ExitOK
}

// PackageManagerVerb prints the package manager, the one internal/sys detects.
func (a *App) PackageManagerVerb(args []string) int {
	if len(args) != 0 {
		return ExitUsage
	}
	manager := a.cfg.Packages.Manager()
	if manager == "" {
		fmt.Fprintln(a.cfg.Stderr, "ryoku-host: no supported package manager found")
		return ExitAbsent
	}
	fmt.Fprintln(a.cfg.Stdout, manager)
	return ExitOK
}
