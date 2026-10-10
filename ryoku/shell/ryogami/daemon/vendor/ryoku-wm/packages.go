package wm

import (
	"os"
	"os/exec"
	"strings"
)

// VariantPackage is the package that makes a provider the desktop's compositor:
// it provides the shared compositor virtual the ryoku-desktop umbrella requires
// and pulls in the provider's satellites. The CLI installs it to switch, and the
// reclaim planner assumes it installed when measuring what leaving the outgoing
// compositor frees.
func VariantPackage(name string) string { return "ryoku-desktop-" + name }

// PackageAvailable reports whether pkg can be installed from an enabled repo.
// It reads the cached metadata only (-C), so a switch preview never blocks on a
// network refresh; dnf repoquery exits 0 on no match, so availability is a
// non-empty answer.
func PackageAvailable(pkg string) bool {
	out, err := dnfCommand("-q", "-C", "repoquery", "--available", pkg).Output()
	return err == nil && strings.TrimSpace(string(out)) != ""
}

func hostPkgInstalled(name string) bool {
	return exec.Command("rpm", "-q", "--quiet", name).Run() == nil
}

// dnfCommand runs dnf in the C locale. The planner parses dnf's section headers
// ("Removing dependent packages:"), which a translated locale would rename.
func dnfCommand(args ...string) *exec.Cmd {
	cmd := exec.Command("dnf", args...)
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	return cmd
}
