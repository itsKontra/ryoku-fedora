package doctor

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"ryoku-cli/internal/sys"
)

// deploy.sh writes some user units straight into ~/.config/systemd/user with
// ExecStart rewritten to ~/.local/bin (ryoku-rashin, for one). Materialize never
// lays those paths, so on a box moved back to packages the copy keeps
// overriding the packaged unit and runs a binary the residue sweep removes.
var (
	packagedUnitDir   = "/usr/lib/systemd/user"
	packagedUnitOwner = sys.PkgOwner
)

// checkoutUnitResidue lists the home user units that point into ~/.local/bin
// and shadow a unit a Ryoku package ships.
func checkoutUnitResidue() []string {
	dir := filepath.Join(sys.ConfigHome(), "systemd", "user")
	localBin := filepath.Join(sys.Home(), ".local", "bin") + "/"
	entries, _ := os.ReadDir(dir)
	var out []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".service") {
			continue
		}
		twin := filepath.Join(packagedUnitDir, e.Name())
		if !sys.Exists(twin) || !strings.HasPrefix(packagedUnitOwner(twin), "ryoku") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil || !execPointsInto(string(b), localBin) {
			continue
		}
		out = append(out, filepath.Join(dir, e.Name()))
	}
	return out
}

func execPointsInto(unit, prefix string) bool {
	for _, line := range strings.Split(unit, "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
		if ok && strings.HasPrefix(k, "Exec") && strings.Contains(v, prefix) {
			return true
		}
	}
	return false
}

// retireCheckoutUnits removes the home units and hands each back to its
// packaged copy: the enablement symlinks still name the deleted file, so every
// unit that was enabled is re-enabled and every running one restarted. Returns
// the paths it could not remove.
func retireCheckoutUnits(units []string) (kept []string) {
	var enabled, active []string
	for _, p := range units {
		name := filepath.Base(p)
		wasEnabled := systemctlUser("is-enabled", "--quiet", name) == nil
		wasActive := systemctlUser("is-active", "--quiet", name) == nil
		if err := os.Remove(p); err != nil {
			kept = append(kept, p)
			continue
		}
		if wasEnabled {
			enabled = append(enabled, name)
		}
		if wasActive {
			active = append(active, name)
		}
	}
	if len(kept) == len(units) {
		return kept
	}
	_ = systemctlUser("daemon-reload")
	if len(enabled) > 0 {
		_ = systemctlUser(append([]string{"reenable"}, enabled...)...)
	}
	if len(active) > 0 {
		_ = systemctlUser(append([]string{"try-restart"}, active...)...)
	}
	return kept
}

var systemctlUser = func(args ...string) error {
	return exec.Command("systemctl", append([]string{"--user"}, args...)...).Run()
}
